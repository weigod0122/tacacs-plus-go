package http

import "sync"

// passwordFailureMu protects both process-local failure maps.  The maps are
// intentionally kept in this package (rather than a distributed store) to
// preserve the existing one-hour behaviour, while making concurrent HTTP
// requests and cleanup safe.
var passwordFailureMu sync.Mutex

func passwordUpdateFailureCount(user string) int8 {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	return updatePasswordErrUser[user]
}

func incrementPasswordUpdateFailure(user string) {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	if updatePasswordErrUser[user] < 127 {
		updatePasswordErrUser[user]++
	}
}

func resetPasswordUpdateFailure(user string) {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	delete(updatePasswordErrUser, user)
}

func clearPasswordUpdateFailures() {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	updatePasswordErrUser = make(map[string]int8)
}

func checkPasswordFailureCount(user string) int8 {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	return checkPasswordErrUser[user]
}

func incrementCheckPasswordFailure(user string) {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	if checkPasswordErrUser[user] < 127 {
		checkPasswordErrUser[user]++
	}
}

func resetCheckPasswordFailure(user string) {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	delete(checkPasswordErrUser, user)
}

func clearCheckPasswordFailures() {
	passwordFailureMu.Lock()
	defer passwordFailureMu.Unlock()
	checkPasswordErrUser = make(map[string]int8)
}
