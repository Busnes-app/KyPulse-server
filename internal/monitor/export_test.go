package monitor

// Flush waits until every delivery queued or held before it has been sent. The sender must
// be running. A marker queued once nothing is held lands behind every refilled delivery.
func Flush(s *Service) {
	for {
		s.mu.Lock()
		held := len(s.overflow)
		s.mu.Unlock()
		done := make(chan struct{})
		s.queue <- delivery{done: done}
		<-done
		if held == 0 {
			return
		}
	}
}
