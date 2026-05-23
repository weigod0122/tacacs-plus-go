package utils

import (
	"reflect"
	"sync"
	"tacacs/pkg/public/log"
	"time"
)

// Semaphore 具有慢启动功能的concurrent.Semaphore
type Semaphore struct {
	maxSize int
	mutex   sync.RWMutex // 使用读写锁
	channel chan int8
}

func NewSlowStartSemaphore(initNum, maxSize int, rampUpTime time.Duration) *Semaphore {
	if initNum > maxSize {
		initNum = maxSize
	}

	s := &Semaphore{
		maxSize: maxSize,
		channel: make(chan int8, initNum),
	}

	// 如果初始值已经是最大值，或者增长时间为0，则不需要慢启动
	if initNum < maxSize && rampUpTime > 0 {
		go s.startRampUp(initNum, maxSize, rampUpTime)
	}

	return s
}

func NewSemaphore(num int) *Semaphore {
	return NewSlowStartSemaphore(num, num, 0)
}

func (s *Semaphore) startRampUp(initNum, maxSize int, rampUpTime time.Duration) {
	increments := maxSize - initNum
	if increments <= 0 {
		return
	}

	start := time.Now()
	// 计算每次增加的间隔时间
	interval := rampUpTime / time.Duration(increments)
	currentSize := initNum

	for i := 0; i < increments; i++ {
		time.Sleep(interval)
		currentSize++

		s.mutex.Lock()
		// 创建新的更大容量的channel
		newChannel := make(chan int8, currentSize)
		oldChannel := s.channel

		// 计算当前已使用的令牌数
		usedTokens := len(oldChannel)

		// 将旧channel中的所有令牌复制到新channel
		for j := 0; j < usedTokens; j++ {
			token := <-oldChannel
			newChannel <- token
		}

		// 替换channel
		s.channel = newChannel
		s.mutex.Unlock()

		// 关闭旧channel，确保不会有新的操作进入
		close(oldChannel)
	}
	log.Logger.Infof("startRampUp: initNum(%v), maxSize(%v), rampUpTime(%v), interval(%v), start at %v", initNum, maxSize, rampUpTime, interval, start.Format(time.DateTime))
}

func (s *Semaphore) TryAcquire() bool {
	s.mutex.RLock()
	ch := s.channel
	s.mutex.RUnlock()

	select {
	case ch <- int8(0):
		return true
	default:
		return false
	}
}

func (s *Semaphore) Acquire() {
	backoff := 1 * time.Millisecond
	maxBackoff := 100 * time.Millisecond
	for {
		s.mutex.RLock()
		ch := s.channel
		s.mutex.RUnlock()

		select {
		case ch <- int8(0):
			return
		default:
			// 如果channel已关闭或满了，重试
			time.Sleep(1 * time.Millisecond)
		}

		time.Sleep(backoff)
		backoff = time.Duration(float64(backoff) * 1.5)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (s *Semaphore) Release() {
	for {
		s.mutex.RLock()
		ch := s.channel
		s.mutex.RUnlock()

		select {
		case <-ch:
			return
		default:
			if reflect.ValueOf(ch).IsNil() {
				return
			}
			// 如果channel已关闭或空了，重试
			time.Sleep(1 * time.Millisecond)
		}
	}
}

func (s *Semaphore) AvailablePermits() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return cap(s.channel) - len(s.channel)
}

func (s *Semaphore) CurrentCapacity() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return cap(s.channel)
}

func (s *Semaphore) MaxCapacity() int {
	return s.maxSize
}
