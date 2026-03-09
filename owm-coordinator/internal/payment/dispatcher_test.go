package payment

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/owmnetwork/owm-coordinator/internal/observer"
)

func TestDispatcher_ObserverFailureDoesNotAffectPayment(t *testing.T) {
	var callCount atomic.Int32

	d := &Dispatcher{
		db:       nil,
		ln:       nil,
		observer: nil,
		log:      zap.NewNop(),
		queue:    make(chan paymentJob, 1),
		SubmitFn: func(_ context.Context, _ observer.Receipt) (string, error) {
			callCount.Add(1)
			return "", fmt.Errorf("observer api 500")
		},
	}

	taskID := uuid.New()
	nodeLNURI := "03abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab@somehost.example:9735"

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.submitObserverReceipt(context.Background(), taskID, nodeLNURI, 500, "preimage_abc")
	}()
	wg.Wait()

	if got := callCount.Load(); got != 1 {
		t.Errorf("expected SubmitFn to be called exactly once, got %d", got)
	}
}
