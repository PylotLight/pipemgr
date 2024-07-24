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
	"github.com/pylotlight/adoMonitor/internal/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type ADOMonitor struct {
	client         *azuredevops.Connection
	buildClient    build.Client
	approvClient   pipelinesapproval.Client
	ctx            context.Context
	pipelines      []string
	project        string
	updateInterval time.Duration
	statusChan     chan types.PipelineStatus
	StatusMap      map[string]string
	observers      []types.Observer
	statuses       []types.PipelineStatus
	statusMu       sync.RWMutex
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
		statusChan:     make(chan types.PipelineStatus, 100),
		observers:      make([]types.Observer, 0),
	}, nil
}

func (m *ADOMonitor) AddPipelines(pipelines []string) {
	m.pipelines = pipelines
}

func (m *ADOMonitor) SetUpdateInterval(interval time.Duration) {
	m.updateInterval = interval
}

func (m *ADOMonitor) Register(o types.Observer) {
	m.observers = append(m.observers, o)
}

func (m *ADOMonitor) Unregister(o types.Observer) {
	for i, observer := range m.observers {
		if observer == o {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			break
		}
	}
}
func (m *ADOMonitor) NotifyAll() {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	for _, observer := range m.observers {
		observer.Update(m.statuses)
	}
}

func (m *ADOMonitor) MonitorPipelines(ctx context.Context) {
	go m.updatePipelineStatuses(ctx)
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
			newStatuses := make([]types.PipelineStatus, 0)
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
					m.statusMu.Lock()
					newStatuses = append(newStatuses, status)
					m.statusMu.Unlock()
				}(url)
			}
			wg.Wait()

			m.statusMu.Lock()
			m.statuses = newStatuses
			m.statusMu.Unlock()

			m.NotifyAll()
		}
	}
}

func (m *ADOMonitor) PerformAction(ApprovalRecordID *uuid.UUID) error {
	// fmt.Printf("Performing action on pipeline %s\n", ApprovalRecordID)
	approver := "approved by person in x"
	params := pipelinesapproval.UpdateApprovalsArgs{
		Project: &m.project,
		UpdateParameters: &[]pipelinesapproval.ApprovalUpdateParameters{
			{
				ApprovalId: ApprovalRecordID,
				Status:     &pipelinesapproval.ApprovalStatusValues.Approved,
				Comment:    &approver,
			},
		},
	}
	approv, err := m.approvClient.UpdateApprovals(
		m.ctx,
		params,
	)
	if err != nil {
		return err
	}
	for _, app := range *approv {
		println(app.Status)
	}

	return nil
}

func (m *ADOMonitor) fetchPipelineStatus(BuildId *int) (types.PipelineStatus, error) {

	build, err := m.buildClient.GetBuild(m.ctx, build.GetBuildArgs{
		Project: &m.project,
		BuildId: BuildId,
	})
	if err != nil {
		return types.PipelineStatus{}, fmt.Errorf("error fetching build %d: %v", *BuildId, err)
	}

	resultString := ""
	if build.Result != nil {
		resultString = string(*build.Result)
	}

	status := types.PipelineStatus{
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

func (m *ADOMonitor) fetchPipelineTimeline(BuildId *int, status types.PipelineStatus) (types.PipelineStatus, error) {
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

			stage := []types.StageStatus{
				{
					ID:     *record.Id,
					Order:  *record.Order,
					Name:   *record.Name,
					Status: cases.Title(language.English).String(string(*record.State)),
					Result: cases.Title(language.English).String(resultString),
				},
			}
			if *record.State == "pending" {
				approval := m.processApprovals(&record, timeline.Records)
				if approval != nil {
					if len(*approval.Steps) == 1 {
						appovalStep := (*approval.Steps)[0]
						ApproverGroupName := appovalStep.AssignedApprover.DisplayName
						stage[0].ApprovalID = approval.Id
						resultString = fmt.Sprintf("Awaiting Approval %s", *ApproverGroupName)
					}

					// fmt.Printf("%+v", approval)
				}
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
