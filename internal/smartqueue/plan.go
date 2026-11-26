package smartqueue

import (
	"sync"
	"time"
)

// Status is the assignment cell for one job in a published plan.
type Status string

const (
	StatusAssigned     Status = "assigned"
	StatusWaiting      Status = "waiting"
	StatusIncompatible Status = "incompatible"
	StatusMerged       Status = "merged"
	StatusUnreadable   Status = "unreadable"
)

// Row is one job's assignment in the plan.
type Row struct {
	PrinterID   string
	PrinterName string
	Status      Status
}

// Plan is one published assignment of jobs to printers.
type Plan struct {
	Enabled bool
	Err     string
	Rows    map[string]Row
}

// PlanJob is one pending job. A non-empty ParentJobID is a merged-plate child.
// CreatedAt is RFC3339.
type PlanJob struct {
	ID          string
	ParentJobID string
	Priority    int
	CreatedAt   string
}

// PlanGroup is one group-matching group.
// Edges come from the representative's verdicts and checks.
type PlanGroup struct {
	MemberIDs        []string
	RepresentativeID string
	Verdicts         []Verdict
	Checks           map[string][]Check
}

// PlanInput is everything BuildPlan needs to classify jobs.
type PlanInput struct {
	Jobs         []PlanJob
	PrinterNames map[string]string
	Groups       []PlanGroup
	HalfLife     time.Duration
	Now          time.Time
	Prioritized  map[string]struct{}
}

// BuildPlan classifies each job into a plan row.
// Merged children and jobs with an unparseable createdAt are not matching vertices.
// Merged wins when both apply. Enabled and Err stay zero for the publisher to set.
func BuildPlan(in PlanInput) Plan {
	rows := make(map[string]Row, len(in.Jobs))
	vertices := make([]Job, 0, len(in.Jobs))
	vertex := make(map[string]struct{}, len(in.Jobs))

	for _, job := range in.Jobs {
		if job.ParentJobID != "" {
			rows[job.ID] = Row{Status: StatusMerged}
			continue
		}
		created, err := time.Parse(time.RFC3339, job.CreatedAt)
		if err != nil {
			rows[job.ID] = Row{Status: StatusUnreadable}
			continue
		}
		vertices = append(vertices, Job{
			ID:       job.ID,
			Priority: job.Priority,
			Weight:   Weight(in.Now.Sub(created), in.HalfLife),
		})
		vertex[job.ID] = struct{}{}
	}

	var edges []Edge
	for _, group := range in.Groups {
		printers := edgePrinters(group)
		for _, memberID := range group.MemberIDs {
			if _, ok := vertex[memberID]; !ok {
				continue
			}
			for _, printerID := range printers {
				edges = append(edges, Edge{JobID: memberID, PrinterID: printerID})
			}
		}
	}

	assigned := Assign(vertices, edges, in.Prioritized)
	hasEdge := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		hasEdge[edge.JobID] = struct{}{}
	}
	for _, job := range vertices {
		if printerID, paired := assigned[job.ID]; paired {
			rows[job.ID] = assignedRow(in.PrinterNames, printerID)
			continue
		}
		if _, edged := hasEdge[job.ID]; edged {
			rows[job.ID] = Row{Status: StatusWaiting}
			continue
		}
		rows[job.ID] = Row{Status: StatusIncompatible}
	}
	return Plan{Rows: rows}
}

func edgePrinters(group PlanGroup) []string {
	seen := make(map[string]struct{})
	var printers []string
	for _, edge := range Edges(group.RepresentativeID, group.Verdicts, group.Checks) {
		if _, ok := seen[edge.PrinterID]; ok {
			continue
		}
		if !Eligible(group.Verdicts, edge.PrinterID) {
			continue
		}
		seen[edge.PrinterID] = struct{}{}
		printers = append(printers, edge.PrinterID)
	}
	return printers
}

func assignedRow(names map[string]string, printerID string) Row {
	name := names[printerID]
	if name == "" {
		name = printerID
	}
	return Row{PrinterID: printerID, PrinterName: name, Status: StatusAssigned}
}

// PlanCache stores the latest published plan.
type PlanCache struct {
	mu   sync.Mutex
	plan *Plan
}

// Publish stores a deep copy of the plan.
func (c *PlanCache) Publish(plan Plan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := clonePlan(plan)
	c.plan = &copied
}

// Snapshot returns a deep copy of the published plan.
// A nil plan is what Snapshot returns before the first Publish.
// The queue page renders a nil plan as Calculating.
func (c *PlanCache) Snapshot() *Plan {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.plan == nil {
		return nil
	}
	copied := clonePlan(*c.plan)
	return &copied
}

func clonePlan(plan Plan) Plan {
	return Plan{
		Enabled: plan.Enabled,
		Err:     plan.Err,
		Rows:    cloneRows(plan.Rows),
	}
}

func cloneRows(rows map[string]Row) map[string]Row {
	if rows == nil {
		return nil
	}
	out := make(map[string]Row, len(rows))
	for id, row := range rows {
		out[id] = row
	}
	return out
}
