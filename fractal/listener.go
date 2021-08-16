package fractal

import (
	"context"
	"net"

	"massnet.org/mass/fractal/connection"
)

type Listener struct {
	nl   net.Listener
	opts []connection.Option
}

func NewListener(network, address string, opts ...connection.Option) (*Listener, error) {
	nl, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	l := &Listener{
		nl:   nl,
		opts: opts,
	}
	return l, nil
}

func (l *Listener) Accept() (*connection.Conn, context.CancelFunc, error) {
	c, err := l.nl.Accept()
	if err != nil {
		return nil, nil, err
	}
	var opts []connection.Option
	opts = append(opts, l.opts...)
	opts = append(opts, connection.WithNetConn(c))
	return connection.NewConn(opts...)
}

func (l *Listener) Addr() net.Addr {
	return l.nl.Addr()
}

func (l *Listener) Close() error {
	return l.nl.Close()
}
