package response

const (
	Success          = 200
	Accepted         = 202
	ParamError       = 400
	NoAuth           = 401
	Forbidden        = 403
	NotFound         = 404
	InternalError    = 500
	Failed           = 501
	ServiceUnaval    = 503
)

var messageMap = map[int]string{
	Success:       "success",
	Accepted:      "accepted",
	ParamError:    "parameter error",
	NoAuth:        "token error",
	Forbidden:     "forbidden",
	NotFound:      "not found",
	InternalError: "internal server error",
	Failed:        "failed",
	ServiceUnaval: "service unavailable",
}

type Body struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

func JSON(code int, msg interface{}, data interface{}) Body {
	text := ""
	if msg != nil {
		text = stringify(msg)
	}
	if text == "" {
		text = messageMap[code]
	}
	return Body{
		Code: code,
		Msg:  text,
		Data: data,
	}
}

func stringify(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case error:
		return val.Error()
	default:
		if v == nil {
			return ""
		}
		return ""
	}
}
