package service

import (
	"errors"
	"sync/atomic"
)

var (
	ErrOperating = errors.New("service is operating")
	ErrStarted   = errors.New("service is started")
	ErrStopped   = errors.New("service is stopped")
)

type Service interface {
	Start() error
	OnStart() error
	Stop() error
	OnStop() error
	Started() bool
	Name() string
}

type BaseService struct {
	service   Service
	started   int32
	operating int32
	name      string
}

func NewBaseService(service Service, name string) *BaseService {
	return &BaseService{
		service: service,
		name:    name,
	}
}

func (bs *BaseService) Start() error {
	if swapped := atomic.CompareAndSwapInt32(&bs.operating, 0, 1); !swapped {
		return ErrOperating
	}
	defer atomic.StoreInt32(&bs.operating, 0)

	if atomic.LoadInt32(&bs.started) == 1 {
		return ErrStarted
	}
	if err := bs.service.OnStart(); err != nil {
		return err
	}
	atomic.StoreInt32(&bs.started, 1)

	return nil
}

func (bs *BaseService) OnStart() error {
	return nil
}

func (bs *BaseService) Stop() error {
	if swapped := atomic.CompareAndSwapInt32(&bs.operating, 0, 1); !swapped {
		return ErrOperating
	}
	defer atomic.StoreInt32(&bs.operating, 0)

	if atomic.LoadInt32(&bs.started) == 0 {
		return ErrStopped
	}
	if err := bs.service.OnStop(); err != nil {
		return err
	}
	atomic.StoreInt32(&bs.started, 0)

	return nil
}

func (bs *BaseService) OnStop() error {
	return nil
}

func (bs *BaseService) Started() bool {
	return atomic.LoadInt32(&bs.started) == 1
}

func (bs *BaseService) Name() string {
	return bs.name
}
