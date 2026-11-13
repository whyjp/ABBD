package core

type Speaker interface {
	Speak() string
}

func Run(s Speaker) string {
	return s.Speak()
}
