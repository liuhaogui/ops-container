package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type ContainerService struct {
	client *client.Client
	log    *zap.Logger
}

type LogMessage struct {
	Data string `json:"data"`
	Msg  string `json:"msg"`
	Code int    `json:"code"`
}

type ExecWSMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint   `json:"cols,omitempty"`
	Rows uint   `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
}

func NewContainerService(log *zap.Logger) (*ContainerService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("init docker client: %w", err)
	}

	return &ContainerService{client: cli, log: log}, nil
}

func (s *ContainerService) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *ContainerService) GetContainerList(ctx context.Context) ([]types.Container, error) {
	return s.client.ContainerList(ctx, containertypes.ListOptions{All: true})
}

func (s *ContainerService) ListContainers(ctx context.Context, all bool) ([]types.Container, error) {
	return s.client.ContainerList(ctx, containertypes.ListOptions{All: all})
}

func (s *ContainerService) StopContainer(ctx context.Context, id string) error {
	return s.client.ContainerStop(ctx, id, containertypes.StopOptions{})
}

func (s *ContainerService) StartContainer(ctx context.Context, id string) error {
	return s.client.ContainerStart(ctx, id, containertypes.StartOptions{})
}

func (s *ContainerService) GetContainerLog(ctx context.Context, id string, lines int) ([]string, error) {
	reader, err := s.client.ContainerLogs(ctx, id, containertypes.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Follow:     false,
		Tail:       fmt.Sprintf("%d", lines),
	})
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return scanLogs(reader)
}

func (s *ContainerService) TailContainerLog(ctx context.Context, id string, ws *websocket.Conn) error {
	reader, err := s.client.ContainerLogs(ctx, id, containertypes.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Follow:     true,
		Tail:       "500",
	})
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, buildLog("", -1, fmt.Sprintf("tail container log error %s", err.Error())))
		return err
	}
	defer reader.Close()
	defer ws.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if err := ws.WriteMessage(websocket.TextMessage, buildLog(scanner.Text(), 200, "")); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, buildLog("", -1, err.Error()))
		return err
	}
	return nil
}

func (s *ContainerService) ExecContainerTerminal(ctx context.Context, id string, cmd []string, cols, rows uint, ws *websocket.Conn) error {
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	recorder, castName, err := NewCastRecorder(id, cols, rows, cmd)
	if err != nil {
		s.log.Warn("create cast recorder failed", zap.String("container_id", id), zap.Error(err))
	}
	if recorder != nil {
		defer func() {
			if err := recorder.Close(); err != nil {
				s.log.Warn("close cast recorder failed", zap.String("recording", castName), zap.Error(err))
			}
		}()
	}

	size := &[2]uint{rows, cols}
	resp, err := s.client.ContainerExecCreate(ctx, id, containertypes.ExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		ConsoleSize:  size,
		Cmd:          cmd,
	})
	if err != nil {
		return err
	}

	attach, err := s.client.ContainerExecAttach(ctx, resp.ID, containertypes.ExecAttachOptions{
		Tty:         true,
		ConsoleSize: size,
	})
	if err != nil {
		return err
	}
	defer attach.Close()
	defer ws.Close()

	writeMu := &sync.Mutex{}
	send := func(message ExecWSMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return ws.WriteJSON(message)
	}

	errCh := make(chan error, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := attach.Reader.Read(buf)
			if n > 0 {
				if recorder != nil {
					if err := recorder.RecordOutput(string(buf[:n])); err != nil {
						s.log.Warn("record terminal output failed", zap.String("container_id", id), zap.String("recording", castName), zap.Error(err))
					}
				}
				if err := send(ExecWSMessage{Type: "output", Data: string(buf[:n])}); err != nil {
					errCh <- err
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					errCh <- nil
				} else {
					errCh <- readErr
				}
				return
			}
		}
	}()

	go func() {
		for {
			messageType, payload, readErr := ws.ReadMessage()
			if readErr != nil {
				_ = attach.CloseWrite()
				errCh <- nil
				return
			}

			if messageType == websocket.BinaryMessage {
				if _, err := attach.Conn.Write(payload); err != nil {
					errCh <- err
					return
				}
				continue
			}

			var message ExecWSMessage
			if err := json.Unmarshal(payload, &message); err == nil && message.Type != "" {
				switch message.Type {
				case "resize":
					if err := s.client.ContainerExecResize(ctx, resp.ID, containertypes.ResizeOptions{
						Height: message.Rows,
						Width:  message.Cols,
					}); err != nil {
						errCh <- err
						return
					}
				case "input":
					if _, err := attach.Conn.Write([]byte(message.Data)); err != nil {
						errCh <- err
						return
					}
				}
				continue
			}

			if _, err := attach.Conn.Write(payload); err != nil {
				errCh <- err
				return
			}
		}
	}()

	if err := <-errCh; err != nil {
		_ = send(ExecWSMessage{Type: "error", Data: err.Error(), Code: -1})
		return err
	}

	info, err := s.client.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return err
	}

	return send(ExecWSMessage{Type: "exit", Code: info.ExitCode})
}

func scanLogs(reader io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(reader)
	logs := make([]string, 0)
	for scanner.Scan() {
		logs = append(logs, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return logs, err
	}
	return logs, nil
}

func buildLog(data string, code int, msg string) []byte {
	payload, _ := json.Marshal(LogMessage{
		Data: data,
		Msg:  msg,
		Code: code,
	})
	return payload
}
