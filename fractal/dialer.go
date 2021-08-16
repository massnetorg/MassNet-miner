package fractal

import (
	"context"
	"massnet.org/mass/fractal/connection"
)

type Dialer struct {
	opts []connection.Option
}

func NewDialer(opts ...connection.Option) *Dialer {
	return &Dialer{opts: opts}
}

func (d *Dialer) Dial() (*connection.Conn, context.CancelFunc, error) {
	return connection.NewConn(d.opts...)
}
