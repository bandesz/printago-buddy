package smartqueue

// LowPriority is the Printago priority of a low-lane job.
// Every other priority, including 100, is a normal job.
const LowPriority = 1000

// Job is one side of the assignment. Weight is that job's age weight.
type Job struct {
	ID       string
	Priority int
	Weight   float64
}

// Edge means the job can run on the printer. The edge weight is the job weight.
type Edge struct {
	JobID     string
	PrinterID string
}

// Assign matches jobs to printers by maximum-weight bipartite matching.
// Lanes run in order: normal prioritized, normal, low prioritized, low.
// Each lane uses only printers an earlier lane left free.
// prioritized is the set of prioritized job ids and does not change a job's lane.
// Unmatched jobs are absent from the result.
func Assign(jobs []Job, edges []Edge, prioritized map[string]struct{}) map[string]string {
	out := make(map[string]string)
	used := make(map[string]bool)
	for _, lane := range jobLanes(jobs, prioritized) {
		assignStage(lane, edges, used, out)
	}
	return out
}

func jobLanes(jobs []Job, prioritized map[string]struct{}) [4][]Job {
	var lanes [4][]Job
	for _, job := range jobs {
		i := laneIndex(job, prioritized)
		lanes[i] = append(lanes[i], job)
	}
	return lanes
}

func laneIndex(job Job, prioritized map[string]struct{}) int {
	_, marked := prioritized[job.ID]
	low := job.Priority == LowPriority
	switch {
	case !low && marked:
		return 0
	case !low:
		return 1
	case marked:
		return 2
	default:
		return 3
	}
}

// assignStage matches jobs to printers that are not already used.
// Non-finite weights are replaced with one finite stand-in before scaling,
// then edge weights are scaled by this stage's maximum before solving.
func assignStage(jobs []Job, edges []Edge, used map[string]bool, out map[string]string) {
	if len(jobs) == 0 {
		return
	}
	raw := make([]float64, len(jobs))
	for i, job := range jobs {
		raw[i] = finiteAgeStandIn(job.Weight)
	}
	scaled := ScaleWeights(raw)
	weightOf := make(map[string]float64, len(jobs))
	jobIndex := make(map[string]int, len(jobs))
	for i, job := range jobs {
		weightOf[job.ID] = scaled[i]
		jobIndex[job.ID] = i
	}

	printerIndex := make(map[string]int)
	var printers []string
	seen := make(map[[2]int]struct{})
	var links []link
	for _, edge := range edges {
		ji, ok := jobIndex[edge.JobID]
		if !ok || used[edge.PrinterID] {
			continue
		}
		weight := weightOf[edge.JobID]
		// A non-positive weight must not occupy a printer a later lane could use.
		if weight <= 0 {
			continue
		}
		pi, ok := printerIndex[edge.PrinterID]
		if !ok {
			pi = len(printers)
			printerIndex[edge.PrinterID] = pi
			printers = append(printers, edge.PrinterID)
		}
		key := [2]int{ji, pi}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		links = append(links, link{job: ji, printer: pi, weight: weight})
	}
	if len(links) == 0 {
		return
	}

	for job, printer := range maximumWeightMatching(len(jobs), len(printers), links) {
		out[jobs[job].ID] = printers[printer]
		used[printers[printer]] = true
	}
}

type link struct {
	job, printer int
	weight       float64
}

// maximumWeightMatching returns job index to printer index.
// Hungarian minimization uses negated scaled weights, so the chosen edges
// are a maximum-weight matching. Dummy columns leave jobs unmatched.
func maximumWeightMatching(nJobs, nPrinters int, links []link) map[int]int {
	cols := nPrinters + nJobs
	cost := make([][]float64, nJobs)
	for job := range cost {
		cost[job] = make([]float64, cols)
		for printer := range nPrinters {
			cost[job][printer] = 1
		}
	}
	for _, link := range links {
		cost[link.job][link.printer] = -link.weight
	}

	assigned := make(map[int]int)
	for job, col := range hungarianMin(cost) {
		if col >= 0 && col < nPrinters {
			assigned[job] = col
		}
	}
	return assigned
}

// hungarianMin assigns each row to a distinct column at minimum cost.
// The cost matrix must have at least as many columns as rows.
func hungarianMin(cost [][]float64) []int {
	n := len(cost)
	m := len(cost[0])
	u := make([]float64, n+1)
	v := make([]float64, m+1)
	p := make([]int, m+1)
	way := make([]int, m+1)
	const inf = 1e18
	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, m+1)
		used := make([]bool, m+1)
		for j := range minv {
			minv[j] = inf
		}
		for {
			used[j0] = true
			i0 := p[j0]
			delta := inf
			j1 := 0
			for j := 1; j <= m; j++ {
				if used[j] {
					continue
				}
				cur := cost[i0-1][j-1] - u[i0] - v[j]
				if cur < minv[j] {
					minv[j] = cur
					way[j] = j0
				}
				if minv[j] < delta {
					delta = minv[j]
					j1 = j
				}
			}
			for j := 0; j <= m; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
			if j0 == 0 {
				break
			}
		}
	}
	ans := make([]int, n)
	for i := range ans {
		ans[i] = -1
	}
	for j := 1; j <= m; j++ {
		if p[j] != 0 {
			ans[p[j]-1] = j - 1
		}
	}
	return ans
}
