package crawler

import (
	"fmt"
	"github.com/yaklang/yaklang/common/crawler"
	"io"
	"testing"
)

// 原有 Pikachu 测试用例保持不变
func TestCrawler_Run_Dvwa(t *testing.T) {
	fmt.Println("start crawler: ", "http://192.168.3.4:8777/index.php")

	hasBody := false
	c, err := crawler.NewCrawler(
		"http://192.168.3.4:8777/index.php",
		crawler.WithOnRequest(func(req *crawler.Req) {
			// 增加更详细的日志输出
			request := req.Request()
			buffer := make([]byte, 2048)
			_, err := request.Body.Read(buffer)
			if err != nil && err != io.EOF {
				fmt.Println("read error:", err)
				return
			}
			if len(buffer) > 0 {
				hasBody = true
			}
			fmt.Printf("请求URL: %s, 方法: %s, 请求参数: %s \n", req.Url(), request.Method, string(buffer))
		}),
		crawler.WithProxy("http://127.0.0.1:8888"),
		//WithConcurrent(5),
		crawler.WithFixedCookie("PHPSESSID", "o46pk8kkqvdqd11t1l7t1bauv1"),
		crawler.WithFixedCookie("security", "low"),
		crawler.WithMaxDepth(3),
		crawler.WithMaxRedirectTimes(0),
		// 启用调试日志
	)
	if err != nil {
		t.Fatal(err)
	}
	err = c.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !hasBody {
		t.Fatal("请求体为空")
	}
}
