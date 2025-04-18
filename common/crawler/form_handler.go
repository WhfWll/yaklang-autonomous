package crawler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/multipart"
	"golang.org/x/net/html"
)

var (
	usernameKeyword  = []string{"name", "user", "ming", "id", "xingming", "mingzi", "phone", "mail", "tel", "un", "account"}
	passwordKeyword  = []string{"pass", "word", "mima", "code", "secret", "key", "pw", "pwd", "pd"}
	csrftokenKeyword = []string{"csrf_token", "csrftoken", "token", "user_token"}
)

func AbsoluteURL(u string, base *url.URL) string {
	if strings.HasPrefix(u, "#") {
		return ""
	}

	absURL, err := base.Parse(u)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "//" {
		absURL.Scheme = base.Scheme
	}
	return absURL.String()
}

func tolowerStrip(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func HandleElementForm(dom *goquery.Selection, baseURL *url.URL, guessParams ...func(user, pass string, extra map[string][]string)) (method, requestURL, contentType string, body *bytes.Buffer, err error) {
	action := dom.AttrOr("action", baseURL.Path)
	// 移除 # 以及 # 后面的内容
	if sharpIndex := strings.Index(action, "#"); sharpIndex >= 0 {
		action = action[:sharpIndex]
	}
	actionAbsURL := AbsoluteURL(action, baseURL)
	if actionAbsURL == "" {
		return "", "", "", nil, utils.Errorf("build action absolute url fail")
	}
	enctype := dom.AttrOr("enctype", "application/x-www-form-urlencoded")
	method = dom.AttrOr("method", "get")
	switch strings.TrimSpace(strings.ToUpper(method)) {
	case "GET":
		method = "GET"
		break
	case "POST":
		method = "POST"
		break
	default:
		method = "GET"
	}
	log.Debugf("found form [%s] enctype: %s", method, enctype)

	selects := dom.Find("input,textarea")
	log.Debugf("inputs size: %v", selects.Length())

	switch tolowerStrip(enctype) {
	case "multipart/form-data":
		log.Debug("found a form using multipart/form-data")
		body, contentType, err := HandleMultipartFormData(selects)
		if err != nil {
			return "", "", "", nil, errors.Errorf("analyze multipart form-data failed: %s", err)
		}
		return method, actionAbsURL, contentType, body, nil
	default:
		requestURL, body, contentType, err := HandleFormUrlEncoded(method, actionAbsURL, selects, guessParams...)
		if err != nil {
			return "", "", "", nil, errors.Errorf("analyze form requestURL failed: %s", err)
		}
		return method, requestURL, contentType, body, nil
	}
}

func HandleMultipartFormData(selects *goquery.Selection) (body *bytes.Buffer, contentType string, err error) {
	// 分析表单中的 input
	body = bytes.NewBufferString("")
	mw := multipart.NewWriter(body)

	type part struct {
		IsFile    bool
		FieldName string
		FileName  string
		Value     []string
	}

	for _, inputNode := range selects.Nodes {
		var (
			currentPart = &part{}
		)

		switch strings.TrimSpace(strings.ToLower(inputNode.Data)) {
		case "textarea":
			for _, attr := range inputNode.Attr {
				// splited by node
				switch key := tolowerStrip(attr.Key); key {
				case "name":
					currentPart.FieldName = attr.Val
					continue
				case "value":
					currentPart.Value = append(currentPart.Value, attr.Val)
					continue
				}
			}

			if len(currentPart.Value) <= 0 {
				var raw []string
				for _, c := range getHTMLNodeChildren(inputNode) {
					raw = append(raw, c.Data)
				}
				currentPart.Value = append(currentPart.Value, strings.Join(raw, " "))
			}
		case "input":
			for _, attr := range inputNode.Attr {
				// splited by node
				switch key := tolowerStrip(attr.Key); key {
				case "name":
					currentPart.FieldName = attr.Val
					continue
				case "value":
					currentPart.Value = append(currentPart.Value, attr.Val)
					continue
				case "type":
					if tolowerStrip(attr.Val) == "file" {
						currentPart.IsFile = true
					}
				}
			}
		}

		if currentPart.FieldName == "" {
			continue
		}

		if len(currentPart.Value) <= 0 {
			currentPart.Value = []string{fmt.Sprintf("crawler-%s", utils.RandStringBytes(5))}
		}

		if currentPart.IsFile && currentPart.FileName == "" {
			currentPart.FileName = fmt.Sprintf("%s.jpg", utils.RandStringBytes(5))
		}

		if currentPart.IsFile {
			writer, err := mw.CreateFormFile(currentPart.FieldName, currentPart.FileName)
			if err != nil {
				log.Errorf("create form file failed: %s", err)
				continue
			}

			_, err = writer.Write([]byte(strings.Join(currentPart.Value, " ")))
			if err != nil {
				log.Errorf("write form file content failed: %s", err)
				continue
			}
		} else {
			err := mw.WriteField(currentPart.FieldName, strings.Join(currentPart.Value, " "))
			if err != nil {
				log.Warnf("write key [%s] failed: %s", currentPart.FieldName, err)
				continue
			}
		}

	}
	_ = mw.Close()

	contentType = fmt.Sprintf("multipart/form-data; boundary=%s", mw.Boundary())

	return
}

func HandleFormUrlEncoded(method string, actionAbsURL string, selects *goquery.Selection, guessParams ...func(username, password string, extra map[string][]string)) (requestURL string, body *bytes.Buffer, contentType string, err error) {
	// 分析表单中的 input
	var data = map[string][]string{}
	var maybeUsername, maybePassword string
	for _, inputNode := range selects.Nodes {
		var (
			formKey   string
			formValue []string
		)

		switch strings.TrimSpace(strings.ToLower(inputNode.Data)) {
		case "textarea":
			for _, attr := range inputNode.Attr {
				// splited by node
				switch key := tolowerStrip(attr.Key); key {
				case "name":
					formKey = attr.Val
					continue
				case "value":
					formValue = append(formValue, attr.Val)
					continue
				}
			}
			if len(formValue) <= 0 {
				var raw []string
				for _, c := range getHTMLNodeChildren(inputNode) {
					raw = append(raw, c.Data)
				}
				formValue = append(formValue, strings.Join(raw, " "))
			}
		case "input":
			for _, attr := range inputNode.Attr {
				// splited by node
				switch key := tolowerStrip(attr.Key); key {
				case "name":
					formKey = attr.Val
					continue
				case "value":
					formValue = append(formValue, attr.Val)
					continue
				}
			}
		}

		if formKey == "" {
			continue
		}

		keymap := map[string]string{
			"id":     "1",
			"number": "1",
			"page":   "1",
			"offset": "1",
			"order":  "1",
			"limit":  "1",
			"filter": "1",
			"action": "list",
		}
		for _, u := range usernameKeyword {
			if utils.IContains(formKey, u) && len(formKey) > 2 && !utils.MatchAnyOfSubString(
				formKey, csrftokenKeyword...) {
				maybeUsername = formKey
				continue
			}
			keymap[u] = "admin"
		}
		for _, p := range passwordKeyword {
			if utils.IContains(formKey, p) && len(formKey) > 1 {
				maybePassword = formKey
				continue
			}
			keymap[p] = "123456"
		}
		if len(formValue) <= 0 {
			var flag bool
			for key, value := range keymap {
				if utils.IContains(formKey, key) {
					formValue = []string{value}
					flag = true
					break
				}
			}
			if !flag {
				formValue = []string{fmt.Sprintf("crawler-%s", utils.RandStringBytes(5))}
			}
		}

		data[formKey] = formValue
	}

	// 空表单不作处理
	if len(data) <= 0 {
		return "", nil, "", errors.Errorf("form not inputs")
	}

	query := utils.MapQueryToString(data)
	switch tolowerStrip(method) {
	case "post":
		contentType = "application/x-www-form-urlencoded"
		body = bytes.NewBufferString(query)
		requestURL = actionAbsURL
	default:
		u, err := url.Parse(actionAbsURL)
		if err != nil {
			return "", nil, "", errors.Errorf("url[%s] is invalid: %s", actionAbsURL, err)
		}
		u.RawQuery = query
		requestURL = u.String()
		log.Debugf("create [GET] form request from %s to %s query: %s", actionAbsURL, u.String(), query)
		body = bytes.NewBuffer([]byte{})
	}

	for _, guess := range guessParams {
		guess(maybeUsername, maybePassword, data)
	}

	return
}

func getHTMLNodeChildren(r *html.Node) []*html.Node {
	var children []*html.Node

	child := r.FirstChild
	if child != nil {
		children = append(children, child)

		for {
			n := child.NextSibling
			if n == nil {
				break
			}

			children = append(children, n)
		}
	}
	return children
}

// CreateReqHTMLFormData 根据html 的 表单信息 创建新的请求
func CreateReqHTMLFormData(r *Req, node *html.Node) *Req {
	method := "GET"
	if m := getAttr(node, "method"); m != "" {
		method = strings.ToUpper(m)
	}
	action := getAttr(node, "action")
	formUrl := r.AbsoluteURL(action)
	if formUrl == "" {
		formUrl = r.Url()
		//return nil
	}
	// 收集表单参数
	var params []string
	nodeWalker(node, func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "input" || n.Data == "textarea" || n.Data == "select") {
			name := getAttr(n, "name")
			if name == "" {
				return
			}
			value := getAttr(n, "value")
			if n.Data == "select" {
				if option := n.FirstChild; option != nil {
					value = getAttr(option, "value")
				}
			}
			// Generate default value if empty based on route info
			if value == "" {
				value = generateDefaultParamValue(name, r.Url())
			}
			params = append(params, fmt.Sprintf("%s=%s", name, url.QueryEscape(value)))
		}
	})
	// 根据请求方法处理参数
	var body io.Reader
	contentType := "application/x-www-form-urlencoded"
	// 拼接参数
	paramStr := strings.Join(params, "&")
	if method == "GET" {
		// GET请求将参数拼接到URL
		if strings.Contains(formUrl, "?") {
			formUrl += "&" + paramStr
		} else {
			formUrl += "?" + paramStr
		}
		body = http.NoBody
	} else {
		// POST请求处理不同编码类型
		if enctype := getAttr(node, "enctype"); enctype != "" {
			contentType = enctype
			if strings.Contains(enctype, "multipart/form-data") {
				body = createMultipartBody(params)
			} else {
				body = strings.NewReader(paramStr)
			}
		} else {
			body = strings.NewReader(paramStr)
		}
	}

	// 创建并提交请求
	fReq, err := createReqFromUrlEx(r, method, formUrl, body, nil)
	if err != nil {
		log.Errorf("create form request failed: %s", err)
		return nil
	}
	fReq.isForm = true
	fReq.request.Header.Set("Content-Type", contentType)
	fReq.request.Header.Set("Referer", r.request.URL.String())

	lowerBody := strings.ToLower(utils.InterfaceToString(body)) + strings.ToLower(formUrl)
	fReq.maybeLoginForm = utils.MatchAnyOfSubString(
		lowerBody,
		"user", "name", "mail", "id", "xingming", "phone", "unique",
	) && utils.MatchAnyOfSubString(
		lowerBody,
		"pass", "word", "mima", "code", "secret", "key", "passwd", "pw", "pwd", "pd",
	)
	fReq.maybeUploadForm = utils.MatchAllOfRegexp(contentType, `application\/form-data`)
	fReq.depth = r.depth
	return fReq
}

// 定义辅助函数
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func nodeWalker(n *html.Node, f func(*html.Node)) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		f(c)
		nodeWalker(c, f)
	}
}

func createMultipartBody(params []string) io.Reader {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, param := range params {
		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			continue
		}
		writer.WriteField(parts[0], parts[1])
	}
	writer.Close()
	return &buf
}

// generateDefaultParamValue creates context-aware default values based on parameter name and route
func generateDefaultParamValue(name, path string) string {
	pathParts := strings.Split(path, "/")
	var numericParts []string
	for _, part := range pathParts {
		if _, err := strconv.Atoi(part); err == nil {
			numericParts = append(numericParts, part)
		}
	}

	// 从路径中提取数字参数
	if len(numericParts) > 0 {
		return numericParts[0]
	}

	// 常见参数模式匹配
	keymap := map[string]string{
		"id":     "1",
		"number": "1",
		"page":   "1",
		"offset": "1",
		"order":  "1",
		"limit":  "1",
		"filter": "1",
		"action": "list",
	}

	if _, ok := keymap[name]; ok {
		return keymap[name]
	}

	// 用户名密码相关参数
	for _, u := range usernameKeyword {
		if utils.IContains(name, u) && len(name) > 2 {
			return "admin"
		}
		keymap[u] = "admin"
	}
	for _, p := range passwordKeyword {
		if utils.IContains(name, p) && len(name) > 1 {
			return "123456"
		}
		keymap[p] = "123456"
	}

	// CSRF token处理
	if utils.MatchAnyOfSubString(name, csrftokenKeyword...) {
		return utils.RandStringBytes(16)
	}

	// 匹配预定义参数
	for key, value := range keymap {
		if utils.IContains(name, key) {
			return value
		}
	}

	// 默认生成随机值
	return fmt.Sprintf("crawler-%s", utils.RandStringBytes(5))
}
