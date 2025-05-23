package test

import (
	"embed"
	"fmt"
	"github.com/stretchr/testify/require"
	tj "github.com/yaklang/yaklang/common/yak/java/template2java"
	"io/fs"
	"testing"
)

func TestJSP2Java_Content(t *testing.T) {
	tests := []struct {
		name    string
		jspCode string
		wants   []string
	}{
		{
			"test  JspElementWithOpenTagOnly pure text  ",
			"<html>",
			[]string{
				"out = request.getOut(); ",
				`out.write("<html");`,
				`out.write(">");`,
			},
		},
		{
			"test JspElementWithClosingTagOnly pure text  ",
			"<html/>",
			[]string{
				`out.write("<html");`,
			},
		},
		{
			"test JspElementWithTagAndContent pure text  ",
			"<title>hello</title>",
			[]string{
				`out.write("<title");`,
				`out.write(">");`,
				`out.write("hello");`,
				`out.write("</title>");`,
			},
		},
		{
			"test jsp java script",
			"<%\n    int sum = 5 + 10;\n    out.println(\"Sum is: \" + sum);\n%>",
			[]string{
				"int sum = 5 + 10;",
				"out.println(\"Sum is: \" + sum);",
			},
		},
		{
			"test jsp expression script",
			`<%= request.getParameter("userInput") %>`,
			[]string{
				`out.print( request.getParameter("userInput") )`,
			},
		},
		{
			"test jsp declaration script",
			`<%! int count = 0; %>`,
			[]string{
				`int count = 0;`,
			},
		},
		{
			"test jsp directive script import",
			`<%@ page import="java.util.*, com.example.model.User" %>`,
			[]string{
				`import  com.example.model.User;`,
				`import java.util.*;`},
		},
		{
			"test el expression in html content",
			`<p>Welcome, Admin! Your user type is: ${sessionScope.userType}</p>`,
			[]string{
				`out.write("Welcome, Admin! Your user type is: ")`,
				`elExpr.parse("sessionScope.userType"`,
			}},
		// core tag
		// core out tag
		{
			"test jstl-core out tag",
			"<c:out value='${name}'/>",
			[]string{
				`out.printWithEscape(elExpr.parse("name"));`,
			},
		},
		{
			"test jstl-core out tag without escaping",
			"<c:out value='${name}' escapeXml=\"false\"/>",
			[]string{
				`out.print(elExpr.parse("name"));`,
			},
		},
		// core set tag
		{
			"test jstl-core set tag",
			"<c:set var='name' value='John'/>",
			[]string{
				`request.setAttribute("name", John);`,
			},
		},
		// core if tag
		{"test jstl-core if tag 1",
			"<c:if test='${age  <  16 }'>Hello John</c:if>",
			[]string{
				`if (elExpr.parse("age  <  16 "))`,
				`out.write("Hello John");`,
			}},
		{"test jstl-core if tag 2",
			` <c:if test="${sessionScope.userType == 'admin'}">
			 <p>Welcome, Admin! Your user type is: ${sessionScope.userType}</p>
			 </c:if>
        `,
			[]string{
				`if (elExpr.parse("sessionScope.userType == 'admin'")) `,
				`out.write("Welcome, Admin! Your user type is: ");`,
				`out.print(elExpr.parse("sessionScope.userType"));`,
			},
		},
		//core choose tag
		{
			"test jstl-core choose tag ",
			`
			<c:choose>
				<c:when test="${valueToSwitch eq 'case1'}">
					Value is case1
				</c:when>
				<c:when test="${valueToSwitch eq 'case2'}">
					Value is case2
				</c:when>
				<c:when test="${valueToSwitch eq 'case3'}">
					Value is case3
				</c:when>
				<c:otherwise>
					Value does not match any case
				</c:otherwise>
			</c:choose>
`,
			[]string{
				"switch (true) {",
				`case elExpr.parse("valueToSwitch eq 'case1'"):`,
				`out.write("					Value is case1");`,
				"default:"},
		},
		{
			"test jstl-core foreach tag ",
			` 
		<c:forEach var="item" items="${items}">
        <li>${item}</li>
   	 	</c:forEach>`,
			[]string{
				"for (Object item : elExpr.parse(\"items\")) {",
				"out.print(elExpr.parse(\"item\"));",
			},
		},
		{
			"test el expression in jstl ",
			`
        <p>日期：<fmt:formatDate value="${now}" pattern="yyyy-MM-dd HH:mm:ss" /></p>
`,
			[]string{
				"out.print(elExpr.parse(\"now\"));",
			},
		},
	}
	check := func(jspCode string, wants []string) {
		codeInfo, err := tj.ConvertTemplateToJava(tj.JSP, jspCode, "test.jsp")
		require.NoError(t, err)
		require.NotNil(t, codeInfo)
		require.NotEqual(t, 0, len(wants))
		fmt.Print(codeInfo.GetContent())
		checkJavaFront(t, codeInfo.GetContent())
		for _, want := range wants {
			require.Contains(t, codeInfo.GetContent(), want, "want: %s", want)
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check(tt.jspCode, tt.wants)
		})
	}
}

//go:embed jspcode/*
var jspDir embed.FS

func TestRealJsp(t *testing.T) {
	dirEntries, err := fs.ReadDir(jspDir, "jspcode")
	require.NoError(t, err)
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			path := "jspcode/" + entry.Name()
			t.Run(path, func(t *testing.T) {
				content, err := fs.ReadFile(jspDir, path)
				require.NoError(t, err)
				codeInfo, err := tj.ConvertTemplateToJava(tj.JSP, string(content), path)
				require.NoError(t, err)
				require.NotNil(t, codeInfo)
				fmt.Println(codeInfo.GetContent())
				checkJavaFront(t, codeInfo.GetContent())
			})
		}
	}
}
