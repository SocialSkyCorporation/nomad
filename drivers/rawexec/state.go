package rawexec

import (
	"sync"
	"time"

	hclog "github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"
	"github.com/hashicorp/nomad/client/driver/executor"
	"github.com/hashicorp/nomad/plugins/drivers"
)

type taskStore struct {
	store map[string]*rawExecTaskHandle
	lock  sync.RWMutex
}

func newTaskStore() *taskStore {
	return &taskStore{store: map[string]*rawExecTaskHandle{}}
}

func (ts *taskStore) Set(id string, handle *rawExecTaskHandle) {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	ts.store[id] = handle
}

func (ts *taskStore) Get(id string) (*rawExecTaskHandle, bool) {
	ts.lock.RLock()
	defer ts.lock.RUnlock()
	t, ok := ts.store[id]
	return t, ok
}

func (ts *taskStore) Delete(id string) {
	ts.lock.Lock()
	defer ts.lock.Unlock()
	delete(ts.store, id)
}

type rawExecTaskHandle struct {
	exec         executor.Executor
	pid          int
	pluginClient *plugin.Client
	logger       hclog.Logger

	// stateLock syncs access to all fields below
	stateLock sync.RWMutex

	task        *drivers.TaskConfig
	procState   drivers.TaskState
	startedAt   time.Time
	completedAt time.Time
	exitResult  *drivers.ExitResult
}

func (h *rawExecTaskHandle) IsRunning() bool {
	return h.procState == drivers.TaskStateRunning
}

func (h *rawExecTaskHandle) run() {

	// since run is called immediatly after the handle is created this
	// ensures the exitResult is initialized so we avoid a nil pointer
	// thus it does not need to be included in the lock
	if h.exitResult == nil {
		h.exitResult = &drivers.ExitResult{}
	}

	ps, err := h.exec.Wait()
	h.stateLock.Lock()
	defer h.stateLock.Unlock()

	if err != nil {
		h.exitResult.Err = err
		h.procState = drivers.TaskStateUnknown
		h.completedAt = time.Now()
		return
	}
	h.procState = drivers.TaskStateExited
	h.exitResult.ExitCode = ps.ExitCode
	h.exitResult.Signal = ps.Signal
	h.completedAt = ps.Time

	// TODO: detect if the task OOMed
}
