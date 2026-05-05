package tool

import (
	"context"
	"encoding/json"
	"time"
)

// CurrentTime 是 L0 风险的内置工具：返回当前时间、时区和日期。
// 这个工具本身没有任何副作用、参数也可选；主要用来让模型处理"现在几点"
// 一类时效性问题时不要乱猜。
type CurrentTime struct{}

// NewCurrentTime 构造器。
func NewCurrentTime() *CurrentTime { return &CurrentTime{} }

// 输入参数：允许可选指定 IANA 时区，缺省 Asia/Shanghai。
// 输出：年月日、ISO 时间、星期几、时区、Unix 秒。
var currentTimeInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "timezone": {
      "type": "string",
      "description": "IANA 时区，例如 Asia/Shanghai、UTC。默认 Asia/Shanghai。"
    }
  },
  "additionalProperties": false
}`)

var currentTimeOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "iso": { "type": "string" },
    "date": { "type": "string" },
    "time": { "type": "string" },
    "weekday": { "type": "string" },
    "timezone": { "type": "string" },
    "unix": { "type": "integer" }
  }
}`)

func (CurrentTime) Spec() Spec {
	return Spec{
		Name:         "current_time",
		Description:  "返回当前的日期、时间、星期、时区信息。可选参数 timezone 指定 IANA 时区。",
		Category:     "utility",
		RiskLevel:    RiskL0,
		InputSchema:  currentTimeInputSchema,
		OutputSchema: currentTimeOutputSchema,
		TimeoutMS:    5000,
		ProviderType: "builtin",
	}
}

// currentTimeArgs 接收工具参数；字段可选。
type currentTimeArgs struct {
	Timezone string `json:"timezone"`
}

func (CurrentTime) Execute(_ context.Context, args json.RawMessage) (any, error) {
	var in currentTimeArgs
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	loc, err := resolveTimezone(in.Timezone)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(loc)
	return map[string]any{
		"iso":      now.Format(time.RFC3339),
		"date":     now.Format("2006-01-02"),
		"time":     now.Format("15:04:05"),
		"weekday":  now.Weekday().String(),
		"timezone": loc.String(),
		"unix":     now.Unix(),
	}, nil
}

// resolveTimezone 解析用户传入的时区字符串，空字符串走缺省。
func resolveTimezone(tz string) (*time.Location, error) {
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	return time.LoadLocation(tz)
}
