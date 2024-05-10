package loadlimiter

type Strategy interface{}

type shedAll struct{}

var _ Strategy = &shedAll{}

func NewShedAllStrategy() Strategy {
	return &shedAll{}
}
