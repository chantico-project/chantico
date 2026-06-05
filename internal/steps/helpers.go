package steps

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Define a custom type for the Action
type Action int

// Declare the possible Action values using iota
const (
	ActionContinue Action = iota // zero value: proceed
	ActionStop                   // halt, no requeue (a Watch will wake us)
	ActionRequeue                // halt, requeue after duration
	ActionError                  // halt, surface error to controller-runtime
)

type StepResult struct {
	Action       Action
	RequeueAfter time.Duration
	Err          error
}

func Continue() StepResult {
	return StepResult{
		Action: ActionContinue,
	}
}

func Stop() StepResult {
	return StepResult{
		Action: ActionStop,
	}
}

func Error(err error) StepResult {
	return StepResult{
		Action: ActionError,
		Err:    err,
	}
}

func Requeue(duration time.Duration) StepResult {
	return StepResult{
		Action:       ActionRequeue,
		RequeueAfter: duration,
	}
}

type Func[T client.Object] func(ctx context.Context, obj T) StepResult

func Run[T client.Object](ctx context.Context, obj T, steps ...Func[T]) (ctrl.Result, error) {
	for _, step := range steps {
		res := step(ctx, obj)
		switch res.Action {
		case ActionContinue:
			continue
		case ActionStop:
			return ctrl.Result{}, nil
		case ActionRequeue:
			return ctrl.Result{RequeueAfter: res.RequeueAfter}, nil
		case ActionError:
			return ctrl.Result{}, res.Err
		}
	}
	return ctrl.Result{}, nil
}
