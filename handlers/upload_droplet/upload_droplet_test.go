package upload_droplet_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/cloudfoundry-incubator/file-server/handlers"
	"github.com/cloudfoundry-incubator/runtime-schema/router"
	ts "github.com/cloudfoundry/gunk/test_server"
	"github.com/cloudfoundry/gunk/urljoiner"
	"github.com/pivotal-golang/lager"

	. "github.com/cloudfoundry-incubator/file-server/handlers/test_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("UploadDroplet", func() {
	var (
		fakeCloudController *ts.Server
		ccUrl               string
		postStatusCode      int
		postResponseBody    string
		uploadedBytes       []byte
		uploadedFileName    string
		uploadedHeaders     http.Header
		timeClicker         chan time.Time
		startTime           time.Time
		endTime             time.Time

		incomingRequest  *http.Request
		outgoingResponse *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		timeClicker = make(chan time.Time, 4)
		uploadedBytes = nil
		uploadedFileName = ""
		uploadedHeaders = nil

		fakeCloudController = ts.New()
		ccUrl = fakeCloudController.URL()

		fakeCloudController.Append(ts.CombineHandlers(
			ts.VerifyRequest("POST", "/staging/droplets/app-guid/upload", "async=true"),
			ts.VerifyBasicAuth("bob", "password"),
			ts.RespondPtr(&postStatusCode, &postResponseBody),
			func(w http.ResponseWriter, r *http.Request) {
				uploadedHeaders = r.Header
				file, fileHeader, err := r.FormFile("upload[droplet]")
				Ω(err).ShouldNot(HaveOccurred())
				uploadedBytes, err = ioutil.ReadAll(file)
				Ω(err).ShouldNot(HaveOccurred())
				uploadedFileName = fileHeader.Filename
				Ω(r.ContentLength).Should(BeNumerically(">", len(uploadedBytes)))
			},
		))

		var err error
		buffer := bytes.NewBufferString("the file I'm uploading")
		incomingRequest, err = http.NewRequest("POST", "http://file-server.com/v1/droplet/app-guid", buffer)
		incomingRequest.Header.Set("Content-MD5", "the-md5")
		Ω(err).ShouldNot(HaveOccurred())
	})

	JustBeforeEach(func(done Done) {
		conf := handlers.Config{
			CCAddress:            fakeCloudController.URL(),
			CCUsername:           "bob",
			CCPassword:           "password",
			CCJobPollingInterval: 100 * time.Millisecond,
		}

		logger := lager.NewLogger("fakelogger")
		r, err := router.NewFileServerRoutes().Router(handlers.New(conf, logger))
		Ω(err).ShouldNot(HaveOccurred())

		outgoingResponse = httptest.NewRecorder()
		startTime = time.Now()
		r.ServeHTTP(outgoingResponse, incomingRequest)
		endTime = time.Now()
		close(done)
	})

	AfterEach(func() {
		fakeCloudController.Close()
		postStatusCode = 0
		postResponseBody = ""
	})

	Context("uploading the file, when there is no polling", func() {
		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "finished", ccUrl)
		})

		It("should not wait for the polling interval", func() {
			Ω(endTime.Sub(startTime)).Should(BeNumerically("<", 75*time.Millisecond))
		})
	})

	Context("uploading the file, when all is well", func() {

		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "queued", ccUrl)
			fakeCloudController.Append(
				VerifyPollingRequest("my-job-guid", "queued", timeClicker),
				VerifyPollingRequest("my-job-guid", "running", timeClicker),
				VerifyPollingRequest("my-job-guid", "finished", timeClicker),
			)
		})

		It("calls all the requests", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(4))
		})

		It("responds with 201 CREATED", func() {
			Ω(outgoingResponse.Code).Should(Equal(http.StatusCreated))
		})

		It("forwards the content-md5 header", func() {
			Ω(uploadedHeaders.Get("Content-MD5")).Should(Equal("the-md5"))
		})

		It("uploads the correct file", func() {
			Ω(uploadedBytes).Should(Equal([]byte("the file I'm uploading")))
			Ω(uploadedFileName).Should(Equal("droplet.tgz"))
		})

		It("should wait between polls", func() {
			firstTime := <-timeClicker
			secondTime := <-timeClicker
			thirdTime := <-timeClicker

			Ω(secondTime.Sub(firstTime)).Should(BeNumerically(">", 75*time.Millisecond))
			Ω(thirdTime.Sub(secondTime)).Should(BeNumerically(">", 75*time.Millisecond))
		})
	})

	Context("uploading the file, when the job fails", func() {

		BeforeEach(func() {
			postStatusCode = http.StatusCreated
			postResponseBody = PollingResponseBody("my-job-guid", "queued", ccUrl)
			fakeCloudController.Append(
				VerifyPollingRequest("my-job-guid", "queued", timeClicker),
				VerifyPollingRequest("my-job-guid", "running", timeClicker),
				VerifyPollingRequest("my-job-guid", "failed", timeClicker),
				VerifyPollingRequest("my-job-guid", "finished", timeClicker),
			)
		})

		It("stops polling after the first fail", func() {
			Ω(fakeCloudController.ReceivedRequestsCount()).Should(Equal(4))
		})

		It("responds with 500", func() {
			Ω(outgoingResponse.Code).Should(Equal(http.StatusInternalServerError))
		})
	})

	ItFailsWhenTheContentLengthIsMissing(&incomingRequest, &outgoingResponse, &fakeCloudController)
	ItHandlesCCFailures(&postStatusCode, &outgoingResponse, &fakeCloudController)
})

func PollingResponseBody(jobGuid, status string, baseUrl string) string {
	url := urljoiner.Join("/v2/jobs", jobGuid)
	if baseUrl != "" {
		url = urljoiner.Join(baseUrl, url)
	}
	return fmt.Sprintf(`
				{
					"metadata":{
						"guid": "%s",
						"url": "%s"
					},
					"entity": {
						"status": "%s"
					}
				}
			`, jobGuid, url, status)
}

func VerifyPollingRequest(jobGuid, status string, timeClicker chan time.Time) http.HandlerFunc {
	return ts.CombineHandlers(
		ts.VerifyRequest("GET", urljoiner.Join("/v2/jobs/", jobGuid)),
		ts.Respond(http.StatusOK, PollingResponseBody(jobGuid, status, "")),
		func(w http.ResponseWriter, r *http.Request) {
			timeClicker <- time.Now()
		},
	)
}
