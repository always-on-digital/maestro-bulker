package api_based

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	bulker "github.com/jitsucom/bulker/bulkerlib"
	types2 "github.com/jitsucom/bulker/bulkerlib/types"
	"github.com/jitsucom/bulker/jitsubase/appbase"
	"github.com/jitsucom/bulker/jitsubase/utils"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const MixpanelBulkerTypeId = "mixpanel"
const MixpanelUnsupported = "Only 'batch' mode is supported"

var retryDelaysMs = [5]int{100, 200, 200, 500, 0}

func init() {
	bulker.RegisterBulker(MixpanelBulkerTypeId, NewMixpanelBulker)
}

type MixpanelConfig struct {
	ProjectId              string `mapstructure:"projectId" json:"projectId" yaml:"projectId"`
	ServiceAccountUserName string `mapstructure:"serviceAccountUserName" json:"serviceAccountUserName" yaml:"serviceAccountUserName"`
	ServiceAccountPassword string `mapstructure:"serviceAccountPassword" json:"serviceAccountPassword" yaml:"serviceAccountPassword"`
}
type MixpanelBulker struct {
	appbase.Service
	config     MixpanelConfig
	httpClient *http.Client

	closed *atomic.Bool
}

func NewMixpanelBulker(bulkerConfig bulker.Config) (bulker.Bulker, error) {
	mixpanelConfig := MixpanelConfig{}
	if err := utils.ParseObject(bulkerConfig.DestinationConfig, &mixpanelConfig); err != nil {
		return nil, fmt.Errorf("failed to parse destination config: %v", err)
	}
	httpClient := &http.Client{
		Timeout: time.Duration(5) * time.Second,
	}
	return &MixpanelBulker{Service: appbase.NewServiceBase(MixpanelBulkerTypeId), config: mixpanelConfig, httpClient: httpClient,
		closed: &atomic.Bool{}}, nil
}

func (mp *MixpanelBulker) CreateStream(id, tableName string, mode bulker.BulkMode, streamOptions ...bulker.StreamOption) (bulker.BulkerStream, error) {
	switch mode {
	case bulker.Stream:
		return nil, errors.New(MixpanelUnsupported)
	case bulker.Batch:
		return NewTransactionalStream(id, mp, tableName, streamOptions...)
	case bulker.ReplaceTable:
		return nil, errors.New(MixpanelUnsupported)
	case bulker.ReplacePartition:
		return nil, errors.New(MixpanelUnsupported)
	}
	return nil, fmt.Errorf("unsupported bulk mode: %s", mode)
}

func (mp *MixpanelBulker) Type() string {
	return MixpanelBulkerTypeId
}

func (mp *MixpanelBulker) Upload(reader io.Reader, eventsName string, _ int, _ map[string]any) (statusCode int, respBody string, err error) {
	if mp.closed.Load() {
		return 0, "", fmt.Errorf("attempt to use closed Mixpanel instance")
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return 0, "", fmt.Errorf("failed to read request body: %v", err)
	}
	for _, retryDelayMs := range retryDelaysMs {
		var req *http.Request
		//bytes reader
		req, err = http.NewRequest("POST", "https://api.mixpanel.com/import?strict=1&project_id="+mp.config.ProjectId, bytes.NewReader(body))
		if err != nil {
			return 0, "", err
		}
		req.Header.Set("Content-Type", "application/x-ndjson")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		serviceAccount := fmt.Sprintf("%s:%s", mp.config.ServiceAccountUserName, mp.config.ServiceAccountPassword)
		req.Header.Set("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(serviceAccount))))

		var res *http.Response
		res, err = mp.httpClient.Do(req)
		if err != nil {
			statusCode = 0
			respBody = ""
			time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)
			continue
		} else {
			defer res.Body.Close()
			var bodyBytes []byte
			bodyBytes, err = io.ReadAll(res.Body)
			respBody = string(bodyBytes)
			statusCode = res.StatusCode
			errText := ""
			if err != nil {
				errText = err.Error()
			}
			switch statusCode {
			case 200:
				return statusCode, respBody, nil
			case 400:
				if strings.Contains(respBody, "some data points in the request failed validation") {
					return statusCode, respBody, nil
				} else {
					return statusCode, respBody, mp.NewError("http status: %v%s", statusCode, errText)
				}
			case 500, 502, 503:
				err = mp.NewError("http status: %v%s", statusCode, errText)
				time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)
				continue
			default:
				return statusCode, respBody, mp.NewError("http status: %v%s", statusCode, errText)
			}
		}
	}
	return
}

func (mp *MixpanelBulker) GetBatchFileFormat() types2.FileFormat {
	return types2.FileFormatNDJSON
}
func (mp *MixpanelBulker) GetBatchFileCompression() types2.FileCompression {
	return types2.FileCompressionGZIP
}

func (mp *MixpanelBulker) InmemoryBatch() bool {
	return true
}

func (mp *MixpanelBulker) Close() error {
	mp.closed.Store(true)
	mp.httpClient.CloseIdleConnections()
	return nil
}
