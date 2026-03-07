package plan

import "fmt"

type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
)

type Item struct {
	Step   string
	Status Status
}

type State struct {
	Items []Item
}

func New() *State {
	return &State{Items: []Item{}}
}

func (s *State) Add(step string) {
	s.Items = append(s.Items, Item{Step: step, Status: Pending})
}

func (s *State) Set(index int, status Status) error {
	if index < 1 || index > len(s.Items) {
		return fmt.Errorf("index out of range")
	}
	if status == InProgress {
		for i := range s.Items {
			if s.Items[i].Status == InProgress {
				s.Items[i].Status = Pending
			}
		}
	}
	s.Items[index-1].Status = status
	return nil
}

func (s *State) Render() string {
	if len(s.Items) == 0 {
		return "(empty plan)"
	}
	out := ""
	for i, item := range s.Items {
		out += fmt.Sprintf("%d. [%s] %s\n", i+1, item.Status, item.Step)
	}
	return out
}
