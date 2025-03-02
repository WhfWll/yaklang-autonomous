package crawler

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestCrawler_Run(t *testing.T) {
	crawler, err := NewCrawler(
		"http://192.168.3.4:8777/index.php",
		WithOnRequest(func(req *Req) {
			// 增加更详细的日志输出
			println("请求URL: %s, 方法: %s, 深度: %d", req.Url(), req.request.Method, req.depth)
			// 打印Cookie信息
			if len(req.request.Cookies()) > 0 {
				var cookies []string
				for _, cookie := range req.request.Cookies() {
					cookies = append(cookies, cookie.String())
				}
				println("请求携带的Cookie: %s", strings.Join(cookies, "; "))
			}
			// 检查是否有重定向
			if req.response != nil && (req.response.StatusCode == 301 || req.response.StatusCode == 302) {
				println("检测到重定向: %s -> %s", req.Url(), req.response.Header.Get("Location"))
			}
		}),
		WithProxy("http://192.168.22.1:8888"),
		WithFixedCookie("PHPSESSID", "pfjrmm42ofoagssvovc2mlpm05"),
		WithFixedCookie("security", "low"),
		WithMaxRedirectTimes(0),
		// 启用调试日志
	)
	if err != nil {
		t.Fatal(err)
	}
	err = crawler.Run()
	if err != nil {
		t.Fatal(err)
	}
}

type buildHttpRequestTestCase struct {
	req         []byte
	https       bool
	urlString   string
	rsp         []byte
	expectHttps bool
	expectReq   []byte
	noPacket    bool
}

func TestNewHTTPRequest(t *testing.T) {
	baseReq := []byte("GET / HTTP/1.1\r\nHost: www.example.com\r\n\r\n")

	testcases := []*buildHttpRequestTestCase{
		{
			req:         baseReq,
			https:       true,
			urlString:   "//baidu.com/abc",
			rsp:         nil,
			expectHttps: true,
			expectReq:   []byte("GET /abc HTTP/1.1\r\nHost: baidu.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
		{
			req:       baseReq,
			https:     true,
			urlString: "javascript:void(0)",
			rsp:       nil,
			noPacket:  true,
		},
		{
			req:         baseReq,
			https:       true,
			urlString:   "http://baidu.com/abc",
			rsp:         nil,
			expectHttps: false,
			expectReq:   []byte("GET /abc HTTP/1.1\r\nHost: baidu.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
		{
			req:         baseReq,
			https:       true,
			urlString:   "/abc",
			rsp:         nil,
			expectHttps: true,
			expectReq:   []byte("GET /abc HTTP/1.1\r\nHost: www.example.com\r\nReferer: https://www.example.com/\r\n\r\n"),
		},
	}

	for _, testcase := range testcases {
		builtHttps, builtReq, err := NewHTTPRequest(testcase.https, testcase.req, testcase.rsp, testcase.urlString)
		if testcase.noPacket {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, testcase.expectHttps, builtHttps)
			require.Equal(t, string(testcase.expectReq), string(builtReq))
		}
	}

}
