package uploader_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"throughputramp/uploader"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("Uploader", func() {

	Describe("Validate", func() {
		It("accepts endpoint-only configs", func() {
			c := &uploader.Config{
				Endpoint:        "endpoint",
				AccessKeyID:     "A",
				SecretAccessKey: "B",
				BucketName:      "C",
			}
			err := c.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("accepts region-only configs", func() {
			c := &uploader.Config{
				AwsRegion:       "region",
				AccessKeyID:     "A",
				SecretAccessKey: "B",
				BucketName:      "C",
			}
			err := c.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("fails when no region or endpoint is provided", func() {
			c := &uploader.Config{
				AccessKeyID:     "A",
				SecretAccessKey: "B",
				BucketName:      "C",
			}
			err := c.Validate()
			Expect(err).To(MatchError("S3 region or endpoint is required."))
		})

		It("fails when bucket name is empty", func() {
			c := &uploader.Config{
				Endpoint:        "endpoint",
				AccessKeyID:     "A",
				SecretAccessKey: "B",
			}
			err := c.Validate()
			Expect(err).To(MatchError("S3 bucket is required."))
		})

		It("fails when access key ID is empty", func() {
			c := &uploader.Config{
				Endpoint:        "endpoint",
				SecretAccessKey: "B",
				BucketName:      "C",
			}
			err := c.Validate()
			Expect(err).To(MatchError("AccessKeyID is required."))
		})

		It("fails when secret access key is empty", func() {
			c := &uploader.Config{
				Endpoint:    "endpoint",
				AccessKeyID: "A",
				BucketName:  "C",
			}
			err := c.Validate()
			Expect(err).To(MatchError("SecretAccessKey is required."))
		})
	})

	Describe("Upload", func() {
		var (
			testS3Server *ghttp.Server
			bucketName   string
			fileName     string
			uploadConfig *uploader.Config
			file         *bytes.Buffer
		)

		BeforeEach(func() {
			testS3Server = ghttp.NewServer()
			testS3Server.AppendHandlers(ghttp.RespondWith(http.StatusBadGateway, "error-uploading"))
			testS3Server.AppendHandlers(ghttp.RespondWith(http.StatusBadGateway, "error-uploading"))
			testS3Server.AppendHandlers(ghttp.RespondWith(http.StatusBadGateway, "error-uploading"))

			fileName = "testfile"
			bucketName = "blah-bucket"
			file = bytes.NewBufferString("test body")

			uploadConfig = &uploader.Config{
				BucketName:      bucketName,
				Endpoint:        testS3Server.URL(),
				AccessKeyID:     "ABCD",
				SecretAccessKey: "ABCD",
			}
		})

		AfterEach(func() {
			testS3Server.Close()
		})

		Context("with no content type specified", func() {
			var (
				bodyChan chan []byte
			)
			BeforeEach(func() {
				bodyChan = make(chan []byte, 1)
				testS3Server.AppendHandlers(ghttp.CombineHandlers(
					ghttp.VerifyRequest("PUT", "/"+bucketName+"/"+fileName),
					ghttp.VerifyHeaderKV("X-Amz-Acl", "public-read"),
					func(rw http.ResponseWriter, req *http.Request) {
						defer GinkgoRecover()
						defer req.Body.Close()
						bodyBytes, err := ioutil.ReadAll(req.Body)
						Expect(err).ToNot(HaveOccurred())
						bodyChan <- bodyBytes
					},
					ghttp.RespondWith(http.StatusOK, nil),
				))
			})
			AfterEach(func() {
				close(bodyChan)
			})
			It("can upload a publicly-readable file S3 with retries", func() {
				dest, err := uploader.Upload(uploadConfig, file, fileName, false)
				Expect(err).ToNot(HaveOccurred())
				Expect(dest).To(Equal(testS3Server.URL() + "/" + bucketName + "/" + fileName))
				var bodyBytes []byte
				Eventually(bodyChan).Should(Receive(&bodyBytes))
				Expect(string(bodyBytes)).To(Equal("test body"))
			})
		})
		Context("with a content type specified", func() {
			BeforeEach(func() {
				testS3Server.AppendHandlers(ghttp.VerifyContentType("image/png"))
			})
			It("can upload a publicly-readable file S3 with retries", func() {
				dest, err := uploader.Upload(uploadConfig, file, fileName, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(dest).To(Equal(testS3Server.URL() + "/" + bucketName + "/" + fileName))
			})

		})
	})
})
