package lang

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantOne []string // 命中其中任一即视为通过；空字符串代表期望识别失败
	}{
		{
			name:    "go",
			src:     "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n",
			wantOne: []string{"go"},
		},
		{
			name:    "python",
			src:     "def hello(name):\n    print(f'hello {name}')\n\nhello('world')\n",
			wantOne: []string{"python", "python 2"},
		},
		{
			name:    "javascript",
			src:     "const add = (a, b) => a + b;\nconsole.log(add(1, 2));\n",
			wantOne: []string{"javascript", "typescript"},
		},
		{
			name: "chinese-with-shell-commands-must-not-be-bash",
			src: `执行 tcamp 每日 code review 任务：
1. 获取今天日期 YYYYMMDD（如 20260428）
2. mkdir -p /data/workspace/tcamp/report
3. 拉取最新代码：cd /data/workspace/tcamp/camp-common && git pull`,
			wantOne: []string{"markdown", ""},
		},
		{
			// 中英混合的 markdown 笔记：含 **bold** + autolink + ![img] + > 引用
			// CJK 占比 < 30%，过去走不进 markdown 分支，被错判 Code。
			name: "mixed-cjk-markdown-with-strong-signals",
			src: `折腾了挺长时间，终于把这个小工具做到了一个自己觉得能用的状态，拿出来分享一下。 > GoPaste 是一款轻量、安全、开源的剪贴板管理工具，自动记录复制历史并本地加密存储，按快捷键即可秒搜秒粘，支持 Windows、macOS 和 Linux。 > > 项目地址：**<https://github.com/GoPaste/GoPaste>** > > ![gopaste-poster](https://example.com/poster.png)`,
			wantOne: []string{"markdown"},
		},
		{
			// 纯英文的 markdown README：靠 # 标题 + - 列表 + ``` 围栏命中
			name: "english-markdown-readme",
			src: "# GoPaste\n\nA lightweight clipboard manager.\n\n## Features\n\n- cross platform\n- encrypted storage\n\n```bash\nmake dev\n```\n",
			wantOne: []string{"markdown"},
		},
		{
			// 防回归：C 的 #include 单独出现不能被误判 markdown
			name: "c-include-must-not-be-markdown",
			src:  "#include <stdio.h>\n#include <stdlib.h>\n\nint main(void) {\n    printf(\"hi\\n\");\n    return 0;\n}\n",
			wantOne: []string{"c", "c++"},
		},
		{
			// 防回归：YAML 列表（- 开头）单独出现不能被误判 markdown
			name: "yaml-list-must-not-be-markdown",
			src:  "services:\n  web:\n    image: nginx\n    ports:\n      - 80:80\n      - 443:443\n",
			wantOne: []string{"yaml"},
		},
		{
			// 防回归：JSON 含 URL 字符串字段，不能被 autolink 正则误判
			// （我们的 autolink 要求 <https://...> 完整尖括号包裹）
			name: "json-with-url-must-not-be-markdown",
			src:  "{\n  \"homepage\": \"https://example.com\",\n  \"name\": \"foo\"\n}\n",
			wantOne: []string{"json"},
		},
		{
			// 防回归：以 [ 开头的日志/命令行输出不应被识别为 JSON。
			// 历史 bug：曾经的 JSON 规则里有 `^\s*[{\[]`，任何以方括号开头
			// 的文本（浏览器 console 贴出来的 `[DIAG] ...` 日志、`[INFO] ...`
			// 输出、中文笔记里的 `[备注]` 标签）都会被误判。
			name: "bracket-prefixed-log-must-not-be-json",
			src: `[DIAG] window blur: view= main hasFocus= false
App.vue:876 [DIAG] blur timer: view= main hasFocus= true
App.js:54 Uncaught TypeError: Cannot read properties of undefined (reading 'main')
    at HideWindow (App.js:54:22)`,
			wantOne: []string{"", "javascript", "typescript"},
		},
		{
			// 防回归：浏览器 console 日志曾经被 chroma.Analyse 误判成 Gdscript3
			// 之类的冷门语言。现在第四道防线加了主流语言白名单，应该落到通用 Code。
			name: "browser-console-log-must-not-be-niche-language",
			src: `[DIAG] window blur: view= main hasFocus= false
App.vue:876 [DIAG] blur timer: view= main hasFocus= true
2App.vue:872 [DIAG] window blur: view= main hasFocus= false
App.vue:876 [DIAG] blur timer: view= main hasFocus= false
App.js:54 Uncaught TypeError: Cannot read properties of undefined (reading 'main')
    at HideWindow (App.js:54:22)
    at App.vue:881:7`,
			wantOne: []string{"", "javascript", "typescript"},
		},
		{
			// 防回归：Go 源码里包含大量 markdown 语法字面量（典型：detect_test.go
			// 自己、爬虫里拼 markdown 的代码），markdown 强信号（![](), [](),
			// <http://...>, **bold**）会合法出现在反引号 raw string 里。
			// 必须靠 Go 三件套（package + import + func）抢在 markdown 强检测前命中。
			name: "go-source-with-markdown-literals-must-be-go",
			src: "package lang\n\nimport \"testing\"\n\nfunc TestFoo(t *testing.T) {\n" +
				"\tcases := []string{\n" +
				"\t\t`**<https://github.com/GoPaste/GoPaste>**`,\n" +
				"\t\t`![poster](https://example.com/poster.png)`,\n" +
				"\t\t`[link](https://example.com)`,\n" +
				"\t}\n" +
				"\t_ = cases\n" +
				"}\n",
			wantOne: []string{"go"},
		},
		{
			// 防回归：Python 爬虫代码里拼 markdown 字符串，不能被误判 markdown
			name: "python-source-with-markdown-literals-must-be-python",
			src: "import re\n\ndef render(title, url):\n" +
				"    tpl = f'# {title}\\n\\n![img]({url})\\n\\n**bold**\\n'\n" +
				"    return tpl\n",
			wantOne: []string{"python", "python 2"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Detect(c.src)
			ok := false
			for _, w := range c.wantOne {
				if got == w {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("Detect()=%q, want one of %v", got, c.wantOne)
			}
		})
	}
}
