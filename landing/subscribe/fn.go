package subscribe

// see: https://stackoverflow.com/questions/52075778/what-does-a-production-ready-google-cloud-function-look-like

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/uncounch/cloudfunctions/common/rest"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/logging"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
	"google.golang.org/genproto/googleapis/api/monitoredres"
)

var (
	db     *firestore.Client
	logger *logging.Logger
)

// Subscriber is the db model for the mailing list subscriber
type Subscriber struct {
}

type httpSignupFormRequest struct {
	Email         string `json:"email"`
	TermsAccepted bool   `json:"termsAccepted"`
}

type subscriber struct {
	Email         string    `firestore:"email"`
	TermsAccepted bool      `firestore:"termsAccepted"`
	CreatedAt     time.Time `firestore:"createdAt"`
}

// Subscribe registers a user interest in a future product
func Subscribe(w http.ResponseWriter, r *http.Request) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer logger.Flush()

		logger.Log(logging.Entry{
			Payload:  "Handling new HTTP request",
			Severity: logging.Debug,
		})

		if r.Method != http.MethodPost {
			rest.WriteHTTPStatus(w, http.StatusMethodNotAllowed)
			return
		}

		if r.Body == nil {
			rest.WriteHTTPStatus(w, http.StatusBadRequest)
			return
		}

		var data httpSignupFormRequest

		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}

		s := &subscriber{
			Email:         data.Email,
			TermsAccepted: data.TermsAccepted,
			CreatedAt:     time.Now().UTC(),
		}

		logger.Log(logging.Entry{
			Payload:  "Storing new subscriber",
			Severity: logging.Debug,
		})

		if _, err := db.Collection("subscribers").Doc(s.Email).Set(r.Context(), s); err != nil {
			logger.Log(logging.Entry{
				Payload:  err,
				Severity: logging.Error,
			})

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rest.WriteHTTPStatus(w, http.StatusOK)
	}
	traced := &ochttp.Handler{
		Handler: http.HandlerFunc(fn),
	}
	traced.ServeHTTP(w, r)
}

func init() {
	projectID := os.Getenv("GCP_PROJECT")
	if projectID == "" {
		panic(fmt.Errorf("GCP_PROJECT environment variable unset or missing"))
	}

	functionName := os.Getenv("FUNCTION_NAME")
	if functionName == "" {
		panic(fmt.Errorf("FUNCTION_NAME environment variable unset or missing"))
	}

	region := os.Getenv("FUNCTION_REGION")
	if region == "" {
		panic(fmt.Errorf("FUNCTION_REGION environment variable unset or missing"))
	}

	var err error

	// initialize trace exporter
	traceExporter, err := stackdriver.NewExporter(stackdriver.Options{ProjectID: projectID})
	if err != nil {
		panic(fmt.Errorf("error initializing tracer: %s", err))
	}

	trace.RegisterExporter(traceExporter)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	// configure logger
	logClient, err := logging.NewClient(context.Background(), projectID)
	if err != nil {
		panic(fmt.Errorf("error initializing logger: %s", err))
	}

	monitoredResource := monitoredres.MonitoredResource{
		Type: "cloud_function",
		Labels: map[string]string{
			"function_name": functionName,
			"region":        region,
		},
	}

	commonResource := logging.CommonResource(&monitoredResource)
	logger = logClient.Logger(functionName, commonResource)

	// initialize firestore client
	db, err = firestore.NewClient(context.Background(), projectID)
	if err != nil {
		panic(err)
	}
}
