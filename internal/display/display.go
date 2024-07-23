package display

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/pylotlight/adoMonitor/internal/monitor"
	"github.com/pylotlight/adoMonitor/internal/types"
)

type model struct {
	statuses       []types.PipelineStatus
	pipelineTable  table.Model
	stageTable     table.Model
	focusIndex     int
	pipelineCursor int
	stageCursor    int
	width          int
	height         int
	// actionCallback types.ActionCallback
}

type TUIDisplay struct {
	program *tea.Program
	model   *model
	mu      sync.Mutex
	monitor *monitor.ADOMonitor // Add this line
}

type updateMsg struct{}

func NewTUIDisplay(m *monitor.ADOMonitor) *TUIDisplay {
	model := InitialModel()
	p := tea.NewProgram(model, tea.WithAltScreen())
	return &TUIDisplay{
		program: p,
		model:   &model,
		monitor: m, // Add this line
	}
}

func (d *TUIDisplay) Update(statuses []types.PipelineStatus) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.model.statuses = statuses
	d.program.Send(updateMsg{})
}

func (d *TUIDisplay) Run() error {
	_, err := d.program.Run()
	return err
}

func InitialModel() model {
	pipelineColumns := []table.Column{
		{Title: "Pipeline", Width: 30},
		{Title: "Status", Width: 10},
		{Title: "Result", Width: 10},
		{Title: "Elapsed", Width: 10},
	}

	stageColumns := []table.Column{
		{Title: "Stage", Width: 10},
		{Title: "Status", Width: 10},
		{Title: "Result", Width: 40},
	}

	pipelineTable := table.New(
		table.WithColumns(pipelineColumns),
		table.WithFocused(true),
		table.WithHeight(30),
	)

	stageTable := table.New(
		table.WithColumns(stageColumns),
		table.WithFocused(false),
		table.WithHeight(30),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	pipelineTable.SetStyles(s)
	stageTable.SetStyles(s)

	return model{
		pipelineTable:  pipelineTable,
		stageTable:     stageTable,
		focusIndex:     0,
		pipelineCursor: 0,
		stageCursor:    0,
		width:          100,
		height:         30,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focusIndex = (m.focusIndex + 1) % 2
		case "up", "down":
			if m.focusIndex == 0 {
				m.pipelineTable, cmd = m.pipelineTable.Update(msg)
				m.updateStagesTable()
			} else {
				cursorMov := m.stageTable.Cursor() + 1
				if msg.String() == "up" {
					cursorMov = m.stageTable.Cursor() - 1
				}
				m.stageTable, cmd = m.stageTable.Update(msg)
				m.stageTable.SetCursor(cursorMov)
			}
		case "enter":
			selectedID := m.stageTable.SelectedRow()[0]
			println(selectedID)
			// m.
			// err := m.monitor.PerformAction(selectedID)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.pipelineTable.SetHeight(m.height/2 - 2)
		m.stageTable.SetHeight(m.height/2 - 2)
	case []types.PipelineStatus:
		m.statuses = msg
		m.updateTables()
	}
	return m, cmd
}

func (m *model) updateTables() {
	var pipelineRows []table.Row
	sort.Slice(m.statuses, func(i, j int) bool {
		return m.statuses[i].Name < m.statuses[j].Name
	})
	for _, status := range m.statuses {
		pipelineRows = append(pipelineRows, table.Row{
			status.Name,
			status.Status,
			status.Result,
			status.TimeElapsed,
		})
	}
	m.pipelineTable.SetRows(pipelineRows)
	m.updateStagesTable()
}

func (m *model) updateStagesTable() {
	if len(m.statuses) > 0 {
		selectedPipeline := m.statuses[m.pipelineTable.Cursor()]

		sort.Slice(selectedPipeline.Stages, func(i, j int) bool {
			return selectedPipeline.Stages[i].Order < selectedPipeline.Stages[j].Order
		})

		var stageRows []table.Row
		for _, stage := range selectedPipeline.Stages {
			stageRows = append(stageRows, table.Row{
				stage.Name,
				stage.Status,
				stage.Result,
			})
		}
		m.stageTable.SetRows(stageRows)
	}
}

func (m model) View() string {
	if m.width < 80 || m.height < 15 {
		return "Terminal window too small. Please resize."
	}

	focusedStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("62"))

	unfocusedStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	mainTableWidth := m.width/2 - 4
	stageInfoWidth := m.width/2 - 4

	m.pipelineTable.SetWidth(mainTableWidth)
	m.stageTable.SetWidth(stageInfoWidth)

	var mainTable, stageTable string

	if m.focusIndex == 0 {
		mainTable = focusedStyle.Width(mainTableWidth).Render(m.pipelineTable.View())
		stageTable = unfocusedStyle.Width(stageInfoWidth).Render(m.stageTable.View())
	} else {
		mainTable = unfocusedStyle.Width(mainTableWidth).Render(m.pipelineTable.View())
		stageTable = focusedStyle.Width(stageInfoWidth).Render(m.stageTable.View())
	}

	mainView := lipgloss.JoinHorizontal(lipgloss.Left, mainTable, stageTable)
	// mainView := lipgloss.JoinVertical(lipgloss.Left, mainTable, stageTable)

	help := "\nUse ↑/↓ to navigate. Tab to switch focus. Press q to quit."
	return lipgloss.JoinVertical(lipgloss.Left, mainView, help)
}

func PipelineStatuses(statuses []types.PipelineStatus) {
	p := tea.NewProgram(InitialModel(), tea.WithAltScreen())
	go func() {
		for {
			p.Send(statuses)
			time.Sleep(time.Second)
		}
	}()
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
	}
}
