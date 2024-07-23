package monitor

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/pipelinesapproval"
	"github.com/pylotlight/adoMonitor/internal/config"
	"github.com/pylotlight/adoMonitor/internal/display"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	tea "github.com/charmbracelet/bubbletea"
)

type ADOMonitor struct {
	client         *azuredevops.Connection
	buildClient    build.Client
	approvClient   pipelinesapproval.Client
	ctx            context.Context
	pipelines      []string
	project        string
	updateInterval time.Duration
	statusChan     chan display.PipelineStatus
	StatusMap      map[string]string
}

type RecordInfo struct {
	Record   *build.TimelineRecord
	Children map[uuid.UUID]*build.TimelineRecord
}

func NewADOMonitor(pat, org, project string) (*ADOMonitor, error) {
	ctx := context.Background()
	connection := azuredevops.NewPatConnection(org, pat)
	buildClient, err := build.NewClient(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("error creating build client: %v", err)
	}
	approvalsClient, err := pipelinesapproval.NewClient(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("error creating approvals client: %v", err)
	}
	return &ADOMonitor{
		client:         connection,
		buildClient:    buildClient,
		approvClient:   approvalsClient,
		project:        project,
		ctx:            ctx,
		updateInterval: 10 * time.Second, // API call interval
		statusChan:     make(chan display.PipelineStatus, 100),
	}, nil
}

func (m *ADOMonitor) AddPipelines(pipelines []string) {
	m.pipelines = pipelines
}

func (m *ADOMonitor) SetUpdateInterval(interval time.Duration) {
	m.updateInterval = interval
}

func (m *ADOMonitor) MonitorPipelines(ctx context.Context) {
	go m.updatePipelineStatuses(ctx)
	m.displayUpdates(ctx)
}

func (m *ADOMonitor) updatePipelineStatuses(ctx context.Context) {
	ticker := time.NewTicker(m.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var wg sync.WaitGroup
			for _, url := range m.pipelines {
				wg.Add(1)
				go func(url string) {
					defer wg.Done()
					buildID, err := config.ExtractBuildID(url)
					if err != nil {
						return
					}

					status, err := m.fetchPipelineStatus(&buildID)
					if err != nil {
						fmt.Printf("Error fetching pipeline status: %v\n", err)
						return
					}
					status, err = m.fetchPipelineTimeline(&buildID, status)
					if err != nil {
						fmt.Printf("Error fetching pipeline timeline: %v\n", err)
						return
					}
					m.statusChan <- status
				}(url)
			}
			wg.Wait()
		}
	}
}

func (m *ADOMonitor) fetchPipelineStatus(BuildId *int) (display.PipelineStatus, error) {

	build, err := m.buildClient.GetBuild(m.ctx, build.GetBuildArgs{
		Project: &m.project,
		BuildId: BuildId,
	})
	if err != nil {
		return display.PipelineStatus{}, fmt.Errorf("error fetching build %d: %v", *BuildId, err)
	}

	resultString := ""
	if build.Result != nil {
		resultString = string(*build.Result)
	}

	status := display.PipelineStatus{
		ID:     fmt.Sprintf("%d", *build.Id),
		Name:   fmt.Sprintf("%s - %s", *build.Definition.Name, *build.BuildNumber),
		Status: cases.Title(language.English).String(string(*build.Status)),
		Result: cases.Title(language.English).String(resultString),
	}

	if build.StartTime != nil {
		status.CreatedDate = build.StartTime.String()
		status.TimeElapsed = calculateTimeElapsed(build.StartTime.Time, build.FinishTime)
	}

	if build.FinishTime != nil {
		status.FinishedDate = build.FinishTime.String()
	}

	return status, nil
}

func (m *ADOMonitor) fetchPipelineTimeline(BuildId *int, status display.PipelineStatus) (display.PipelineStatus, error) {
	// Get the timeline for the build
	timeline, err := m.buildClient.GetBuildTimeline(m.ctx, build.GetBuildTimelineArgs{
		Project: &m.project,
		BuildId: BuildId,
	})
	if err != nil {
		log.Fatalf("Error getting build timeline: %v", err)
	}

	for _, record := range *timeline.Records {
		if *record.Type == "Stage" {
			resultString := ""
			if record.Result != nil {
				resultString = strings.ToTitle(string(*record.Result))
			}

			if *record.State == "pending" {
				approval := m.processApprovals(&record, timeline.Records)
				if approval != nil {
					if len(*approval.Steps) == 1 {
						appovalStep := (*approval.Steps)[0]
						ApproverGroupName := appovalStep.AssignedApprover.DisplayName
						resultString = fmt.Sprintf("Awaiting Approval %s", *ApproverGroupName)
					}

					// fmt.Printf("%+v", approval)
				}
			}
			stage := []display.StageStatus{
				{
					ID:     *record.Id,
					Order:  *record.Order,
					Name:   *record.Name,
					Status: cases.Title(language.English).String(string(*record.State)),
					Result: cases.Title(language.English).String(resultString),
				},
			}
			status.Stages = append(status.Stages, stage...)
		}
	}

	return status, nil
}

func (m *ADOMonitor) processApprovals(currentStage *build.TimelineRecord, Records *[]build.TimelineRecord) *pipelinesapproval.Approval {
	// Find Checkpoint record
	var checkpointRecord *build.TimelineRecord
	for _, potentialCheckpoint := range *Records {
		if *potentialCheckpoint.Type == "Checkpoint" && *potentialCheckpoint.ParentId == *currentStage.Id {
			checkpointRecord = &potentialCheckpoint
			break
		}
	}

	if checkpointRecord != nil {
		var approvalRecord *build.TimelineRecord
		for _, potentialApproval := range *Records {
			if *potentialApproval.Type == "Checkpoint.Approval" && *potentialApproval.ParentId == *checkpointRecord.Id {
				approvalRecord = &potentialApproval
				break
			}
		}

		if approvalRecord != nil {
			approval, err := m.approvClient.GetApproval(m.ctx, pipelinesapproval.GetApprovalArgs{
				Project:    &m.project,
				ApprovalId: approvalRecord.Id,
				Expand:     &pipelinesapproval.ApprovalDetailsExpandParameterValues.Steps,
			})
			if err != nil {
				return nil
			}
			return approval
		}
	}

	return nil
}

func calculateTimeElapsed(startTime time.Time, finishTime *azuredevops.Time) string {
	var elapsed time.Duration
	if finishTime != nil {
		elapsed = finishTime.Time.Sub(startTime)
	} else {
		elapsed = time.Since(startTime)
	}
	return fmt.Sprintf("%02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
}

func (m *ADOMonitor) displayUpdates(ctx context.Context) {
	statuses := make(map[string]display.PipelineStatus)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	p := tea.NewProgram(display.InitialModel())
	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Println("Error running program:", err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case status := <-m.statusChan:
			statuses[status.ID] = status
		case <-ticker.C:
			var statusList []display.PipelineStatus
			for _, status := range statuses {
				statusList = append(statusList, status)
			}
			p.Send(statusList)
		}
	}
}
