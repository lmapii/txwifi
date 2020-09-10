// IoT Wifi Management

package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bhoriuchi/go-bunyan/bunyan"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/lmapii/txwifi/iotwifi"
)

// APIReturn structures a message for returned API calls.
type APIReturn struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Payload interface{} `json:"payload"`
}

func main() {

	logConfig := bunyan.Config{
		Name:   "txwifi",
		Stream: os.Stdout,
		Level:  bunyan.LogLevelDebug,
	}

	blog, err := bunyan.CreateLogger(logConfig)
	if err != nil {
		panic(err)
	}

	blog.Info("Starting IoT Wifi...")

	messages := make(chan iotwifi.CmdMessage, 1)

	cfgURL := setEnvIfEmpty("IOTWIFI_CFG", "cfg/wificfg.json")
	port := setEnvIfEmpty("IOTWIFI_PORT", "8080")

	bootDelay, err := strconv.Atoi(setEnvIfEmpty("IOTWIFI_BOOT_DELAY", "0"))
	if err != nil {
		bootDelay = 0
	}
	time.Sleep(time.Duration(bootDelay) * time.Second)

	go iotwifi.RunWifi(blog, messages, cfgURL)
	wpacfg := iotwifi.NewWpaCfg(blog, cfgURL)

	apiPayloadReturn := func(w http.ResponseWriter, message string, payload interface{}) {
		APIReturn := &APIReturn{
			Status:  "OK",
			Message: message,
			Payload: payload,
		}
		ret, _ := json.Marshal(APIReturn)

		w.Header().Set("Content-Type", "application/json")
		w.Write(ret)
	}

	// marshallPost populates a struct with json in post body
	marshallPost := func(w http.ResponseWriter, r *http.Request, v interface{}) {
		bytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			blog.Error(err)
			return
		}

		defer r.Body.Close()

		decoder := json.NewDecoder(strings.NewReader(string(bytes)))

		err = decoder.Decode(&v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			blog.Error(err)
			return
		}
	}

	// common error return from api
	retError := func(w http.ResponseWriter, err error) {
		APIReturn := &APIReturn{
			Status:  "FAIL",
			Message: err.Error(),
		}
		ret, _ := json.Marshal(APIReturn)

		w.Header().Set("Content-Type", "application/json")
		w.Write(ret)
	}

	// handle /status POSTs json in the form of iotwifi.WpaConnect
	statusHandler := func(w http.ResponseWriter, r *http.Request) {

		status, err := wpacfg.Status()
		if err != nil {
			blog.Error(err.Error())
			return
		}

		apiPayloadReturn(w, "status", status)
	}

	// handle /disconnect
	disconnectHandler := func(w http.ResponseWriter, r *http.Request) {
		// ignoring content since we're simply disconnecting all networks
		wpacfg.Disconnect()
		apiPayloadReturn(w, "status", struct{}{})
	}

	// handle /connect POSTs json in the form of iotwifi.WpaConnect
	connectHandler := func(w http.ResponseWriter, r *http.Request) {
		var creds iotwifi.WpaCredentials
		marshallPost(w, r, &creds)

		blog.Info("Connect Handler Got: ssid:|%s| psk:|%s|", creds.Ssid, creds.Psk)

		connection, err := wpacfg.ConnectNetwork(creds)
		if err != nil {
			blog.Error(err.Error())
			return
		}

		APIReturn := &APIReturn{
			Status:  "OK",
			Message: "Connection",
			Payload: connection,
		}

		ret, err := json.Marshal(APIReturn)
		if err != nil {
			retError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(ret)
	}

	// scan for wifi networks
	scanHandler := func(w http.ResponseWriter, r *http.Request) {
		blog.Info("Got Scan")
		wpaNetworks, err := wpacfg.ScanNetworks()
		if err != nil {
			retError(w, err)
			return
		}

		APIReturn := &APIReturn{
			Status:  "OK",
			Message: "Networks",
			Payload: wpaNetworks,
		}

		ret, err := json.Marshal(APIReturn)
		if err != nil {
			retError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(ret)
	}

	// kill the application
	killHandler := func(w http.ResponseWriter, r *http.Request) {
		messages <- iotwifi.CmdMessage{Id: "kill"}

		APIReturn := &APIReturn{
			Status:  "OK",
			Message: "Killing service.",
		}
		ret, err := json.Marshal(APIReturn)
		if err != nil {
			retError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(ret)
	}

	// common log middleware for api
	logHandler := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			staticFields := make(map[string]interface{})
			staticFields["remote"] = r.RemoteAddr
			staticFields["method"] = r.Method
			staticFields["url"] = r.RequestURI

			blog.Info(staticFields, "HTTP")
			next.ServeHTTP(w, r)
		})
	}

	// setup router and middleware
	r := mux.NewRouter()
	r.Use(logHandler)

	// set app routes
	r.HandleFunc("/status", statusHandler)
	r.HandleFunc("/connect", connectHandler).Methods("POST")
	r.HandleFunc("/disconnect", disconnectHandler).Methods("POST")
	r.HandleFunc("/scan", scanHandler)
	r.HandleFunc("/kill", killHandler)
	http.Handle("/", r)

	// CORS
	headersOk := handlers.AllowedHeaders([]string{"Content-Type", "Authorization", "Content-Length", "X-Requested-With", "Accept", "Origin"})
	originsOk := handlers.AllowedOrigins([]string{"*"})
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})

	// serve http
	blog.Info("HTTP Listening on " + port)
	http.ListenAndServe(":"+port, handlers.CORS(originsOk, headersOk, methodsOk)(r))

}

// getEnv gets an environment variable or sets a default if
// one does not exist.
func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}

	return value
}

// setEnvIfEmp<ty sets an environment variable to itself or
// fallback if empty.
func setEnvIfEmpty(env string, fallback string) (envVal string) {
	envVal = getEnv(env, fallback)
	os.Setenv(env, envVal)

	return envVal
}
