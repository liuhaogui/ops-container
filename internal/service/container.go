package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
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

// Docker 暴露内部 docker client，供同进程内其它 service（如 HostService）复用，
// 避免重复打开 dialer。调用方不要负责 Close —— 由 ContainerService.Close 统一关。
func (s *ContainerService) Docker() *client.Client { return s.client }

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

// RestartContainer 触发 docker restart：无论容器当前状态都执行重启。
// 与 stop+start 的语义区别：docker daemon 内部处理停止超时，更接近运维直觉。
func (s *ContainerService) RestartContainer(ctx context.Context, id string) error {
	return s.client.ContainerRestart(ctx, id, containertypes.StopOptions{})
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

	// 协议（与 ops-fe-v2 src/components/Terminal/src/Terminal.vue 一致）：
	//   - 服务端 → 客户端：output / error / exit 均写为 TextMessage 的原始字节，
	//     前端 xterm 直接 term.write(evt.data)。**不再包 JSON 信封**，否则会把
	//     `{"type":"output",...}` 这种字符串显示在终端上。
	//   - 客户端 → 服务端：
	//     * 非 JSON 的 TextMessage / BinaryMessage：直接当 stdin 写入容器（前端
	//       `socket.send(data)` 走这条）
	//     * JSON `{"type":"resize","cols":..,"rows":..}`：调 ContainerExecResize
	//     * JSON `{"type":"input","data":"..."}`：写 data 到 stdin（兼容老协议）
	//     * JSON `{"type":"ping",...}` 或其它已知 type：仅做心跳，不下行 docker
	//     * JSON 但 type 未知或缺失：保守起见**不写 stdin**，避免 ping/控制帧被误写
	writeMu := &sync.Mutex{}
	sendBytes := func(b []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return ws.WriteMessage(websocket.TextMessage, b)
	}

	errCh := make(chan error, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := attach.Reader.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				if recorder != nil {
					if err := recorder.RecordOutput(string(chunk)); err != nil {
						s.log.Warn("record terminal output failed", zap.String("container_id", id), zap.String("recording", castName), zap.Error(err))
					}
				}
				if err := sendBytes(chunk); err != nil {
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

			// 尝试按 JSON 控制帧解析；失败或不是控制帧时再当 stdin 透传。
			if looksLikeJSONObject(payload) {
				var message ExecWSMessage
				if err := json.Unmarshal(payload, &message); err == nil {
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
					default:
						// "ping" / 未知 type：仅作为心跳/控制帧，不下行 docker。
					}
					continue
				}
			}

			if _, err := attach.Conn.Write(payload); err != nil {
				errCh <- err
				return
			}
		}
	}()

	if err := <-errCh; err != nil {
		_ = sendBytes([]byte("\r\n[error] " + err.Error() + "\r\n"))
		return err
	}

	info, err := s.client.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		return err
	}

	return sendBytes([]byte("\r\n[exit code " + strconv.Itoa(info.ExitCode) + "]\r\n"))
}

// looksLikeJSONObject 用 payload 第一个非空白字符是否为 '{' 来判断是否是 JSON 对象。
// 比 json.Unmarshal 失败再 fallback 节省一次反序列化；命中后再做严格解析。
func looksLikeJSONObject(payload []byte) bool {
	for _, b := range payload {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		return b == '{'
	}
	return false
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
