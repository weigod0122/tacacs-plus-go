package waitGroup

import "sync"

var (
	GlobalWg = &sync.WaitGroup{}
)
