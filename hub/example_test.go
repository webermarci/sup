package hub_test

import (
	"fmt"
	"net/http/httptest"
	"strings"

	"github.com/webermarci/sup/hub"
	"github.com/webermarci/sup/rx"
)

func ExampleHub_ServeHTTP() {
	temperature := rx.NewSignal("temperature", 21)
	dashboard := hub.New("dashboard").
		RegisterSignal(temperature)

	request := httptest.NewRequest("GET", "/signals/temperature", nil)
	response := httptest.NewRecorder()
	dashboard.ServeHTTP(response, request)

	fmt.Println(response.Code)
	fmt.Println(strings.TrimSpace(response.Body.String()))

	// Output:
	// 200
	// {"id":"temperature","type":"number","value":21,"writable":false}
}
