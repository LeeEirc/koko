package main

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type SSHCommandTool struct{}

func (st *SSHCommandTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "ssh_command",
		Desc: "Executable for ssh command",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"finished": {
				Desc:     "filter todo items if finished",
				Type:     schema.Boolean,
				Required: false,
			},
		}),
	}, nil
}

func (st *SSHCommandTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// Mock调用逻辑
	return `{"todos": [{"id": "1", "content": "在2024年12月10日之前完成Eino项目演示文稿的准备工作", "started_at": 1717401600, "deadline": 1717488000, "done": false}]}`, nil
}
