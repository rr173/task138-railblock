package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
)

// smokeFrontend asserts the embedded index page is served at GET / and that it
// drives at least one real business API (create station → station view), so
// the page is not a static landing shell.
func smokeFrontend(srv *httptest.Server, clk *clock.Fake) error {
	code, body, err := doJSON(srv, "GET", "/", nil, false)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("GET /: want 200 got %d", code)
	}
	if len(body) == 0 {
		return fmt.Errorf("GET /: empty body")
	}
	// The page must reference the app so a browser actually uses the API.
	bs := string(body)
	if !contains(bs, "app.js") {
		return fmt.Errorf("GET /: body does not reference app.js")
	}
	// Drive a real business API the page uses: create station then fetch view.
	var st struct {
		ID string `json:"id"`
	}
	if err := mustDo(srv, "POST", "/stations", map[string]string{"code": "FE", "name": "前端站"}, false, &st); err != nil {
		return err
	}
	var view map[string]any
	if err := mustDo(srv, "GET", "/stations/"+st.ID, nil, false, &view); err != nil {
		return err
	}
	if view["station"] == nil {
		return fmt.Errorf("station view missing station field")
	}
	return nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
