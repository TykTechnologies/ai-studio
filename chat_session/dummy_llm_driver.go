package chat_session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"math/rand"

	"github.com/tmc/langchaingo/llms"
)

type DummyDriver struct{}

func (d *DummyDriver) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	paragraph := "this is a ten word sentence that should be sent."

	cOptions := llms.CallOptions{}
	for _, option := range options {
		option(&cOptions)
	}

	if cOptions.StreamingFunc != nil {
		go d.FakeChunkedResponse(paragraph, cOptions.StreamingFunc)
	}
	return paragraph, nil
}

func (d *DummyDriver) FakeChunkedResponse(para string, sFunc func(ctx context.Context, chunk []byte) error) {
	x := strings.Split(para, " ")
	for _, c := range x {
		randomNum := rand.Intn(501) + 100
		time.Sleep(time.Duration(randomNum) * time.Millisecond)

		err := sFunc(context.Background(), []byte(c))
		if err != nil {
			fmt.Println("[DUMMY HANDLER STREAMING ERROR]", err)
		}
	}
}
