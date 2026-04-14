package sup

import "context"

type Ref[I comparable, A any, S any, R any] struct {
	PID I
	sys *System
}

func (r Ref[I, A, S, R]) Cast(msg A) {
	val, ok := r.sys.registry.Load(r.PID)
	if !ok {
		return
	}

	proc := val.(*process[A, S, R])

	defer func() {
		recover()
	}()

	proc.mailbox <- envelope[A, S, R]{isCall: false, castMsg: msg}
}

func (r Ref[I, A, S, R]) Call(ctx context.Context, msg S) (res R, err error) {
	val, ok := r.sys.registry.Load(r.PID)
	if !ok {
		return res, ErrProcessNotFound
	}

	proc := val.(*process[A, S, R])

	callCtx := proc.pool.Get().(*callContext[R])
	defer proc.pool.Put(callCtx)

	env := envelope[A, S, R]{
		isCall:  true,
		callMsg: msg,
		replyCh: callCtx.replyCh,
		errCh:   callCtx.errCh,
	}

	func() {
		defer func() {
			if recover() != nil {
				err = ErrProcessNotFound
			}
		}()
		select {
		case proc.mailbox <- env:
		case <-ctx.Done():
			err = ctx.Err()
		}
	}()

	if err != nil {
		return res, err
	}

	select {
	case res = <-callCtx.replyCh:
		err = <-callCtx.errCh
		return res, err
	case <-ctx.Done():
		return res, ctx.Err()
	}
}
