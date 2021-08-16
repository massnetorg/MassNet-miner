package connection

import (
	"context"
	"math"
	"net"
	"time"
)

const (
	defaultKeepaliveTimeout      = 60 * time.Second
	defaultKeepaliveInterval     = 29 * time.Second
	defaultMaxRecvMsgSize        = 2 * 1024 * 1024
	defaultMaxSendMsgSize        = math.MaxUint32
	defaultCopyMsg               = true
	defaultRecvQueueSize         = 10
	defaultSendQueueSize         = 10
	defaultPrioritySendQueueSize = 10
	defaultAlivenessChannelSize  = 10
	defaultDialNetwork           = "tcp"
	defaultDialTimeout           = 20 * time.Second
)

type options struct {
	ctx                   context.Context
	keepaliveTimeout      time.Duration
	keepaliveInterval     time.Duration
	maxRecvMsgSize        uint32
	maxSendMsgSize        uint32
	copyMsg               bool
	recvQueueSize         int
	sendQueueSize         int
	prioritySendQueueSize int
	alivenessChannelSize  int
	conn                  net.Conn
	dialNetwork           string
	dialAddress           string
	dialTimeout           time.Duration
}

func defaultOptions() *options {
	return &options{
		keepaliveTimeout:      defaultKeepaliveTimeout,
		keepaliveInterval:     defaultKeepaliveInterval,
		maxRecvMsgSize:        defaultMaxRecvMsgSize,
		maxSendMsgSize:        defaultMaxSendMsgSize,
		copyMsg:               defaultCopyMsg,
		recvQueueSize:         defaultRecvQueueSize,
		sendQueueSize:         defaultSendQueueSize,
		prioritySendQueueSize: defaultPrioritySendQueueSize,
		alivenessChannelSize:  defaultAlivenessChannelSize,
		dialNetwork:           defaultDialNetwork,
		dialTimeout:           defaultDialTimeout,
	}
}

type Option interface {
	apply(*options)
}

type optionFunc struct {
	fn func(*options)
}

func (of *optionFunc) apply(o *options) {
	of.fn(o)
}

func newOptionFunc(fn func(*options)) *optionFunc {
	return &optionFunc{fn: fn}
}

func WithContext(ctx context.Context) Option {
	return newOptionFunc(func(o *options) {
		o.ctx = ctx
	})
}

func KeepaliveTimeout(timeout time.Duration) Option {
	return newOptionFunc(func(o *options) {
		o.keepaliveTimeout = timeout
	})
}

func KeepaliveInterval(interval time.Duration) Option {
	return newOptionFunc(func(o *options) {
		o.keepaliveInterval = interval
	})
}

func MaxRecvMsgSize(size uint32) Option {
	return newOptionFunc(func(o *options) {
		o.maxRecvMsgSize = size
	})
}

func MaxSendMsgSize(size uint32) Option {
	return newOptionFunc(func(o *options) {
		o.maxSendMsgSize = size
	})
}

func CopyMsg(copyMsg bool) Option {
	return newOptionFunc(func(o *options) {
		o.copyMsg = copyMsg
	})
}

func WithNetConn(conn net.Conn) Option {
	return newOptionFunc(func(o *options) {
		o.conn = conn
	})
}

func DialNetwork(network string) Option {
	return newOptionFunc(func(o *options) {
		o.dialNetwork = network
	})
}

func DialAddress(address string) Option {
	return newOptionFunc(func(o *options) {
		o.dialAddress = address
	})
}

func DialTimeout(timeout time.Duration) Option {
	return newOptionFunc(func(o *options) {
		o.dialTimeout = timeout
	})
}
