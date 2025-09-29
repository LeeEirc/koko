package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// 活动窗格类型
type activePane int

const (
	shellPane activePane = iota
	agentPane
)

// Model 定义
type model struct {
	username   string
	activePane activePane
	width      int
	height     int

	// Shell 相关
	shellInput   textarea.Model
	shellHistory []string
	shellOutput  viewport.Model

	// Agent 对话相关
	agentInput   textarea.Model
	agentHistory []message
	agentOutput  viewport.Model
}

type message struct {
	sender  string // "user" or "agent"
	content string
}

// 初始化模型
func initialModel(username string) model {
	// Shell 输入框
	shellInput := textarea.New()
	shellInput.Placeholder = "$ Enter shell command..."
	shellInput.ShowLineNumbers = false
	shellInput.SetHeight(3)
	shellInput.Focus()

	// Agent 输入框
	agentInput := textarea.New()
	agentInput.Placeholder = "💬 Ask agent anything..."
	agentInput.ShowLineNumbers = false
	agentInput.SetHeight(3)
	agentInput.Blur()

	// Shell 输出视图
	shellOutput := viewport.New(0, 0)
	shellOutput.SetContent("🐚 Shell Ready. Type commands here.\n")

	// Agent 输出视图
	agentOutput := viewport.New(0, 0)
	agentOutput.SetContent("🤖 Agent: Hello! I'm here to help you. Ask me anything!\n")

	return model{
		username:     username,
		activePane:   shellPane,
		shellInput:   shellInput,
		shellOutput:  shellOutput,
		shellHistory: []string{},
		agentInput:   agentInput,
		agentOutput:  agentOutput,
		agentHistory: []message{
			{sender: "agent", content: "Hello! I'm here to help you. Ask me anything!"},
		},
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "tab":
			// 切换窗格
			if m.activePane == shellPane {
				m.activePane = agentPane
				m.shellInput.Blur()
				m.agentInput.Focus()
			} else {
				m.activePane = shellPane
				m.agentInput.Blur()
				m.shellInput.Focus()
			}
			return m, nil

		case "ctrl+l":
			// 清空当前窗格
			if m.activePane == shellPane {
				m.shellOutput.SetContent("🐚 Shell Ready. Type commands here.\n")
				m.shellHistory = []string{}
			} else {
				m.agentOutput.SetContent("🤖 Agent: History cleared.\n")
				m.agentHistory = []message{
					{sender: "agent", content: "History cleared."},
				}
			}
			return m, nil

		case "enter":
			if m.activePane == shellPane {
				// 执行 shell 命令
				input := strings.TrimSpace(m.shellInput.Value())
				if input != "" {
					m.shellHistory = append(m.shellHistory, input)
					output := executeShellCommand(input)

					// 更新输出
					currentContent := m.shellOutput.View()
					newContent := fmt.Sprintf("%s\n$ %s\n%s", currentContent, input, output)
					m.shellOutput.SetContent(newContent)
					m.shellOutput.GotoBottom()

					m.shellInput.Reset()
				}
			} else {
				// Agent 对话
				input := strings.TrimSpace(m.agentInput.Value())
				if input != "" {
					m.agentHistory = append(m.agentHistory, message{sender: "user", content: input})

					// 模拟 Agent 响应
					response := generateAgentResponse(input)
					m.agentHistory = append(m.agentHistory, message{sender: "agent", content: response})

					// 更新输出
					m.agentOutput.SetContent(formatAgentHistory(m.agentHistory))
					m.agentOutput.GotoBottom()

					m.agentInput.Reset()
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// 计算各部分尺寸
		paneWidth := (msg.Width - 3) / 2
		outputHeight := msg.Height - 10

		// 更新 Shell 组件尺寸
		m.shellInput.SetWidth(paneWidth - 2)
		m.shellOutput.Width = paneWidth - 2
		m.shellOutput.Height = outputHeight

		// 更新 Agent 组件尺寸
		m.agentInput.SetWidth(paneWidth - 2)
		m.agentOutput.Width = paneWidth - 2
		m.agentOutput.Height = outputHeight

		return m, nil
	}

	// 更新当前活动的输入框
	if m.activePane == shellPane {
		m.shellInput, cmd = m.shellInput.Update(msg)
	} else {
		m.agentInput, cmd = m.agentInput.Update(msg)
	}
	cmds = append(cmds, cmd)

	// 更新 viewport
	m.shellOutput, cmd = m.shellOutput.Update(msg)
	cmds = append(cmds, cmd)
	m.agentOutput, cmd = m.agentOutput.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// 样式定义
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FF00")).
		Padding(0, 1)

	activeStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF")).
		Padding(1)

	inactiveStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Padding(0, 1)

	// 构建 Shell 窗格
	shellTitle := titleStyle.Render("🐚 Shell")
	shellPaneObj := lipgloss.JoinVertical(
		lipgloss.Left,
		shellTitle,
		m.shellOutput.View(),
		"",
		m.shellInput.View(),
	)

	// 构建 Agent 窗格
	agentTitle := titleStyle.Render("🤖 Agent")
	agentPaneObj := lipgloss.JoinVertical(
		lipgloss.Left,
		agentTitle,
		m.agentOutput.View(),
		"",
		m.agentInput.View(),
	)

	// 应用边框样式
	if m.activePane == shellPane {
		shellPaneObj = activeStyle.Render(shellPaneObj)
		agentPaneObj = inactiveStyle.Render(agentPaneObj)
	} else {
		shellPaneObj = inactiveStyle.Render(shellPaneObj)
		agentPaneObj = activeStyle.Render(agentPaneObj)
	}

	// 组合两个窗格
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		shellPaneObj,
		agentPaneObj,
	)

	// 帮助信息
	help := helpStyle.Render(
		"Tab: Switch | Enter: Execute/Send | Ctrl+L: Clear | Ctrl+C/Esc: Quit",
	)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		fmt.Sprintf("👤 User: %s", m.username),
		content,
		help,
	)
}

// 执行 Shell 命令
func executeShellCommand(command string) string {
	// 简单命令处理
	switch {
	case strings.HasPrefix(command, "echo "):
		return strings.TrimPrefix(command, "echo ")
	case command == "date":
		return time.Now().Format("2006-01-02 15:04:05")
	case command == "pwd":
		dir, _ := os.Getwd()
		return dir
	case command == "whoami":
		return os.Getenv("USER")
	case strings.HasPrefix(command, "ls"):
		cmd := exec.Command("ls", "-la")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return string(output)
	default:
		// 执行真实命令（需要谨慎处理安全性）
		//parts := strings.Fields(command)
		//if len(parts) == 0 {
		//	return ""
		//}
		//cmd := exec.Command(parts[0], parts[1:]...)
		//output, err := cmd.CombinedOutput()
		//if err != nil {
		//	return fmt.Sprintf("Error: %v\n%s", err, string(output))
		//}
		return string("ooooooo")
	}
}

// 生成 Agent 响应（简单模拟）
func generateAgentResponse(input string) string {
	input = strings.ToLower(input)

	switch {
	case strings.Contains(input, "hello") || strings.Contains(input, "hi"):
		return "Hello! How can I assist you today?"
	case strings.Contains(input, "help"):
		return "I can help you with:\n• General questions\n• Command explanations\n• System information\n• And more!"
	case strings.Contains(input, "time"):
		return fmt.Sprintf("Current time is: %s", time.Now().Format("15:04:05"))
	case strings.Contains(input, "weather"):
		return "I'm a demo agent, but in production I could fetch real weather data! ☀️"
	default:
		return fmt.Sprintf("You said: \"%s\". I'm a simple demo agent. Try asking about 'help', 'time', or 'weather'!", input)
	}
}

// 格式化 Agent 对话历史
func formatAgentHistory(history []message) string {
	var builder strings.Builder
	builder.WriteString("🤖 Agent Chat History\n")
	builder.WriteString(strings.Repeat("─", 50) + "\n\n")

	for _, msg := range history {
		if msg.sender == "user" {
			builder.WriteString(fmt.Sprintf("👤 You: %s\n\n", msg.content))
		} else {
			builder.WriteString(fmt.Sprintf("🤖 Agent: %s\n\n", msg.content))
		}
	}

	return builder.String()
}

func SessionHandler(session ssh.Session) {
	ptyReq, winCh, isPty := session.Pty()
	if !isPty {
		fmt.Fprintln(session, "Error: No PTY requested")
		return
	}

	username := session.User()
	if username == "" {
		username = "guest"
	}
	slog.Default().Info("start:", "user", username)
	m := initialModel(username)
	p := tea.NewProgram(m, tea.WithInput(session), tea.WithOutput(session), tea.WithAltScreen())

	go func() {
		p.Send(tea.WindowSizeMsg{
			Width:  ptyReq.Window.Width,
			Height: ptyReq.Window.Height,
		})
		for win := range winCh {
			p.Send(tea.WindowSizeMsg{
				Width:  win.Width,
				Height: win.Height,
			})
		}
	}()

	if _, err := p.Run(); err != nil {
		slog.Default().Error("Error running program", "err", err)
	}

}

func main() {
	buf, err := os.ReadFile("old_rsa")
	if err != nil {
		slog.Default().Error("Error reading old rsa file", "error", err)
		return
	}
	singer, err := gossh.ParsePrivateKey(buf)
	if err != nil {
		slog.Default().Error("Error parsing old rsa file", "error", err)
		return
	}
	addr := "0.0.0.0:2233"
	srv := &ssh.Server{
		Addr: addr,
		PasswordHandler: func(ctx ssh.Context, password string) error {

			return nil
		},
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) error {
			return nil
		},
		Version:     "JumpServer",
		HostSigners: []ssh.Signer{singer},
		Handler:     SessionHandler,
	}
	slog.Default().Info("Starting server", "addr", addr)
	err = srv.ListenAndServe()
	if err != nil {
		slog.Default().Error("Error starting server", "error", err)
	}
}
