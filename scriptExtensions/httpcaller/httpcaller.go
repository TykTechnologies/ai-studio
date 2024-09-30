package httpcaller

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/d5/tengo/v2"
	"github.com/d5/tengo/v2/token"
)

type HttpCaller struct{}

func NewHttpCaller() *HttpCaller {
	return &HttpCaller{}
}

func (*HttpCaller) TypeName() string {
	return "HttpCaller"
}
func (*HttpCaller) String() string {
	return "HttpCaller"
}
func (*HttpCaller) BinaryOp(op token.Token, rhs tengo.Object) (tengo.Object, error) {
	panic("not implemented")
}

func (*HttpCaller) IsFalsy() bool {
	return false
}
func (*HttpCaller) Equals(another tengo.Object) bool {
	return false
}

func (*HttpCaller) IndexGet(index tengo.Object) (value tengo.Object, err error) {
	panic("not implemented")
}

func (*HttpCaller) IndexSet(index, value tengo.Object) error {
	panic("not implemented")
}

func (*HttpCaller) Iterate() tengo.Iterator {
	panic("not implemented")
}

func (*HttpCaller) CanIterate() bool {
	return false
}

func (*HttpCaller) CanCall() bool {
	return true
}

func makeHttpRequest(method string, url string, headers map[string]string, body string) (string, int, error) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		log.Printf("client: could not create request: %s\n", err)
		return "", 500, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("client: could not send request: %s\n", err)
		return "", 500, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return "", 500, err
	}

	return string(b), resp.StatusCode, nil
}

func (a *HttpCaller) cleanString(str string) string {
	return strings.Trim(str, "\"")
}

func (a *HttpCaller) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("expected 4 arguments, got %d", len(args))
	}

	method := a.cleanString(args[0].String())
	url := a.cleanString(args[1].String())
	headers := args[2].(*tengo.Map)
	body := a.cleanString(args[3].String())

	headersMap := make(map[string]string)
	if headers != nil {
		for key, value := range headers.Value {
			headersMap[a.cleanString(key)] = a.cleanString(value.String())
		}
	}

	out, status, err := makeHttpRequest(method, url, headersMap, body)
	return &tengo.Map{
		Value: map[string]tengo.Object{
			"status":   &tengo.Int{Value: int64(status)},
			"response": &tengo.String{Value: out},
		},
	}, err
}

func (*HttpCaller) Copy() tengo.Object {
	return &HttpCaller{}
}
