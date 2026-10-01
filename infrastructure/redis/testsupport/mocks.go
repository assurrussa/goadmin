// Package testsupport provides focused session mocks without an external Redis wrapper.
package testsupport

import (
	"context"
	"reflect"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/mock/gomock"
)

type MockClientContract struct {
	redis.UniversalClient
	ctrl     *gomock.Controller
	recorder *MockClientContractMockRecorder
}
type MockClientContractMockRecorder struct{ mock *MockClientContract }

func NewMockClientContract(ctrl *gomock.Controller) *MockClientContract {
	m := &MockClientContract{ctrl: ctrl}
	m.recorder = &MockClientContractMockRecorder{mock: m}
	return m
}

func (m *MockClientContract) EXPECT() *MockClientContractMockRecorder { return m.recorder }

type MockClientShardContract struct {
	*MockClientContract
	ctrl     *gomock.Controller
	recorder *MockClientShardContractMockRecorder
}
type MockClientShardContractMockRecorder struct{ mock *MockClientShardContract }

func NewMockClientShardContract(ctrl *gomock.Controller) *MockClientShardContract {
	m := &MockClientShardContract{MockClientContract: NewMockClientContract(ctrl), ctrl: ctrl}
	m.recorder = &MockClientShardContractMockRecorder{mock: m}
	return m
}

func (m *MockClientShardContract) EXPECT() *MockClientShardContractMockRecorder { return m.recorder }

func (m *MockClientContract) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockClientContractMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockClientContract)(nil).Close))
}

func (m *MockClientContract) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	m.ctrl.T.Helper()
	varargs := make([]any, 1, 1+len(keys))
	varargs[0] = ctx
	for _, a := range keys {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Del", varargs...)
	ret0, _ := ret[0].(*redis.IntCmd)
	return ret0
}

func (mr *MockClientContractMockRecorder) Del(ctx any, keys ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{ctx}, keys...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Del", reflect.TypeOf((*MockClientContract)(nil).Del), varargs...)
}

func (m *MockClientContract) Get(ctx context.Context, key string) *redis.StringCmd {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", ctx, key)
	ret0, _ := ret[0].(*redis.StringCmd)
	return ret0
}

func (mr *MockClientContractMockRecorder) Get(ctx, key any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockClientContract)(nil).Get), ctx, key)
}

func (m *MockClientContract) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Scan", ctx, cursor, match, count)
	ret0, _ := ret[0].(*redis.ScanCmd)
	return ret0
}

func (mr *MockClientContractMockRecorder) Scan(ctx, cursor, match, count any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock, "Scan", reflect.TypeOf((*MockClientContract)(nil).Scan), ctx, cursor, match, count,
	)
}

func (m *MockClientContract) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Set", ctx, key, value, expiration)
	ret0, _ := ret[0].(*redis.StatusCmd)
	return ret0
}

func (mr *MockClientContractMockRecorder) Set(ctx, key, value, expiration any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock, "Set", reflect.TypeOf((*MockClientContract)(nil).Set), ctx, key, value, expiration,
	)
}

func (m *MockClientShardContract) ForEachShard(ctx context.Context, fn func(context.Context, *redis.Client) error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ForEachShard", ctx, fn)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockClientShardContractMockRecorder) ForEachShard(ctx, fn any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock, "ForEachShard", reflect.TypeOf((*MockClientShardContract)(nil).ForEachShard), ctx, fn,
	)
}
