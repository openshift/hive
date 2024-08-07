// Code generated by MockGen. DO NOT EDIT.
// Source: ./actuator.go

// Package mock is a generated GoMock package.
package mock

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1beta1 "github.com/openshift/api/machine/v1beta1"
	v1 "github.com/openshift/hive/apis/hive/v1"
	logrus "github.com/sirupsen/logrus"
)

// MockActuator is a mock of Actuator interface.
type MockActuator struct {
	ctrl     *gomock.Controller
	recorder *MockActuatorMockRecorder
}

// MockActuatorMockRecorder is the mock recorder for MockActuator.
type MockActuatorMockRecorder struct {
	mock *MockActuator
}

// NewMockActuator creates a new mock instance.
func NewMockActuator(ctrl *gomock.Controller) *MockActuator {
	mock := &MockActuator{ctrl: ctrl}
	mock.recorder = &MockActuatorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockActuator) EXPECT() *MockActuatorMockRecorder {
	return m.recorder
}

// GenerateMachineSets mocks base method.
func (m *MockActuator) GenerateMachineSets(arg0 *v1.ClusterDeployment, arg1 *v1.MachinePool, arg2 logrus.FieldLogger) ([]*v1beta1.MachineSet, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GenerateMachineSets", arg0, arg1, arg2)
	ret0, _ := ret[0].([]*v1beta1.MachineSet)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GenerateMachineSets indicates an expected call of GenerateMachineSets.
func (mr *MockActuatorMockRecorder) GenerateMachineSets(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GenerateMachineSets", reflect.TypeOf((*MockActuator)(nil).GenerateMachineSets), arg0, arg1, arg2)
}
