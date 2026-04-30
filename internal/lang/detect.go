// Package lang 提供基于内容的源代码语言检测能力。
//
// 设计取舍：
//   - 第一道防线：强 markdown 检测（围栏 / 图片 / 链接 / autolink 一票通过，
//     或多类弱信号组合）。前置目的是覆盖"中英混合的 markdown 笔记"场景：
//     这类内容 CJK 占比常 < 30%，过去会落到代码识别链路被错判。
//   - 第二道防线：CJK 字符占比高 → 直接判 markdown（弱规则）/ 空，规避典型
//     误判（比如"中文说明 + 嵌入 git/mkdir 命令"被误识别为 Bash）。
//   - 第三道防线：手写正则启发式覆盖主流 10+ 语言。规则按特异性排序，先匹配的赢。
//   - 第四道防线：chroma.Analyse 内置 lexer 自评分兜底（小众语言比如 Lua/PHP/Erlang）。
//
// 选择不在前端跑 hljs.highlightAuto 是为了：
//  1. 避免列表渲染时每个 code 项 5-30ms 的卡顿（hljs 是同步阻塞）。
//  2. 让识别结果与条目一同持久化，详情/列表两处展示永远一致。
package lang

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2/lexers"
)

// 仅取前若干字节做检测：足以稳定判别语言，且控制极端长文的耗时。
const detectSampleLimit = 4096

// 中文字符占比超过此值时，跳过代码识别，避免把中文笔记误判成 Bash 之类。
const cjkRatioThreshold = 0.30

// Detect 返回源代码的语言标准名（小写，例如 "go" / "python" / "javascript"）。
// 无法识别时返回空串，调用方应展示通用 "Code" 标签。
func Detect(src string) string {
	if src == "" {
		return ""
	}
	sample := src
	if len(sample) > detectSampleLimit {
		sample = sample[:detectSampleLimit]
	}

	// 第一道：强 markdown 检测。中英混合的 markdown 笔记走不进 CJK 分支，
	// 过去会落到代码识别链路被错判通用 Code，这里用强特征前置兜住。
	if looksLikeMarkdownStrong(sample) {
		return "markdown"
	}

	// 第二道：自然语言保护。CJK 主导时用宽松 markdown 规则二次确认，
	// 否则直接放弃语言识别（展示通用 Code 标签）。
	if isMostlyCJK(sample) {
		if looksLikeMarkdown(sample) {
			return "markdown"
		}
		return ""
	}

	// 第三道：手写正则启发式。优先级高于 chroma.Analyse —— chroma 的部分小众
	// lexer（GDScript / Modula-2 等）会对常见 JS/TS 片段过度自信抢匹配，
	// 自维护规则覆盖主流 10+ 语言更可靠。
	if name := heuristicDetect(sample); name != "" {
		return name
	}

	// 第四道：chroma 内置 lexer 自评分兜底（小众语言比如 Lua/PHP/Erlang）。
	if l := lexers.Analyse(sample); l != nil {
		return normalizeChromaName(l.Config().Name)
	}

	return ""
}

// normalizeChromaName 把 chroma 的展示名（"Go" "C++" "JavaScript"）转成小写标准名。
func normalizeChromaName(name string) string {
	return strings.ToLower(name)
}

// ---- 启发式规则 ------------------------------------------------------------

// langRule 一条语言识别规则：命中 must 中任意正则即认定为该语言。
// 规则顺序敏感——更特异的规则放前面，避免被宽松规则抢匹配。
type langRule struct {
	name string
	must []*regexp.Regexp
}

var (
	// 编译期常驻，避免每次 Detect 重复编译正则（regexp.MustCompile 不便宜）。
	heuristicRules = []langRule{
		// Go：package 声明 + import + func 三件套，几乎不会与其它语言混淆。
		{"go", []*regexp.Regexp{
			regexp.MustCompile(`(?m)^package\s+\w+\s*$`),
			regexp.MustCompile(`(?m)^import\s+["(]`),
			regexp.MustCompile(`\bfunc\s+(?:\(\w+\s+\*?\w+\)\s+)?\w+\s*\(`),
		}},
		// Python：def/class + 冒号缩进语法 + 典型导入。
		{"python", []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*def\s+\w+\s*\(.*\)\s*:`),
			regexp.MustCompile(`(?m)^\s*class\s+\w+(?:\(.*\))?\s*:`),
			regexp.MustCompile(`(?m)^\s*from\s+[\w.]+\s+import\s+`),
			regexp.MustCompile(`(?m)^\s*import\s+\w+\s*$`),
			regexp.MustCompile(`\bprint\s*\(`),
		}},
		// Rust：fn + let + 显式生命周期/Result/Option 等典型符号。
		{"rust", []*regexp.Regexp{
			regexp.MustCompile(`\bfn\s+\w+\s*(?:<[^>]+>)?\s*\(`),
			regexp.MustCompile(`\blet\s+(?:mut\s+)?\w+\s*[:=]`),
			regexp.MustCompile(`\b(?:impl|trait|pub\s+fn|use\s+\w+::)`),
		}},
		// TypeScript：JS 之前判，命中 type/interface/类型注解优先。
		{"typescript", []*regexp.Regexp{
			regexp.MustCompile(`\binterface\s+\w+\s*\{`),
			regexp.MustCompile(`(?m)^\s*type\s+\w+\s*=`),
			regexp.MustCompile(`:\s*(?:string|number|boolean|void|any|unknown)\b`),
			regexp.MustCompile(`\bimport\s+(?:type\s+)?\{[^}]+\}\s+from\s+['"]`),
		}},
		// JavaScript：var/let/const + 箭头函数 + console.log。
		{"javascript", []*regexp.Regexp{
			regexp.MustCompile(`\b(?:const|let|var)\s+\w+\s*=`),
			regexp.MustCompile(`=>\s*[{\(]`),
			regexp.MustCompile(`\bconsole\.(?:log|error|warn|info)\s*\(`),
			regexp.MustCompile(`\bfunction\s*\w*\s*\([^)]*\)\s*\{`),
			regexp.MustCompile(`\brequire\s*\(\s*['"][^'"]+['"]\s*\)`),
		}},
		// Java：public class + System.out.println + 包语句。
		{"java", []*regexp.Regexp{
			regexp.MustCompile(`\bpublic\s+(?:static\s+)?(?:final\s+)?class\s+\w+`),
			regexp.MustCompile(`\bSystem\.out\.print(?:ln)?\s*\(`),
			regexp.MustCompile(`(?m)^package\s+[\w.]+\s*;`),
			regexp.MustCompile(`\bpublic\s+static\s+void\s+main\s*\(`),
		}},
		// C/C++：#include + 主函数 + STL 容器。把 cpp 放 c 前面，命中 cpp 特征优先。
		{"c++", []*regexp.Regexp{
			regexp.MustCompile(`#include\s*<(?:iostream|vector|string|map|memory)>`),
			regexp.MustCompile(`\bstd::\w+`),
			regexp.MustCompile(`\b(?:nullptr|template\s*<|class\s+\w+\s*[:{])`),
		}},
		{"c", []*regexp.Regexp{
			regexp.MustCompile(`#include\s*<(?:stdio|stdlib|string|stdint)\.h>`),
			regexp.MustCompile(`\bint\s+main\s*\(\s*(?:void|int\s+\w+\s*,\s*char)`),
			regexp.MustCompile(`\bprintf\s*\(`),
		}},
		// Shell/Bash：shebang 或典型命令组合。
		{"bash", []*regexp.Regexp{
			regexp.MustCompile(`(?m)^#!/(?:usr/)?bin/(?:env\s+)?(?:ba)?sh\b`),
			regexp.MustCompile(`(?m)^\s*(?:if|for|while)\s+\[\[?`),
			regexp.MustCompile(`\$\{[^}]+\}`),
			regexp.MustCompile(`(?m)^\s*(?:echo|grep|awk|sed|cat|cd|ls|mkdir|rm)\s+`),
		}},
		// JSON：花括号包围的 "key": value 结构。
		{"json", []*regexp.Regexp{
			regexp.MustCompile(`^\s*[{\[]`),
			regexp.MustCompile(`"[\w.-]+"\s*:\s*(?:"[^"]*"|\d+|true|false|null|[\[{])`),
		}},
		// YAML：缩进 + key: value，且不含 JSON 的双引号包键。
		{"yaml", []*regexp.Regexp{
			regexp.MustCompile(`(?m)^[a-zA-Z_][\w-]*\s*:\s*\S`),
			regexp.MustCompile(`(?m)^\s*-\s+\w+`),
		}},
		// SQL：典型语句关键字。
		{"sql", []*regexp.Regexp{
			regexp.MustCompile(`(?i)\b(?:select\s+.+\s+from|insert\s+into|update\s+\w+\s+set|create\s+(?:table|index|view))\b`),
		}},
		// HTML：DOCTYPE 或常见标签对。
		{"html", []*regexp.Regexp{
			regexp.MustCompile(`(?i)<!doctype\s+html>`),
			regexp.MustCompile(`(?i)<(?:html|head|body|div|span|p|a|h[1-6])\b`),
		}},
		// CSS：选择器 + 花括号 + property: value;
		{"css", []*regexp.Regexp{
			regexp.MustCompile(`[.#]?\w+\s*\{[^}]*[\w-]+\s*:\s*[^;]+;`),
		}},
	}
)

// heuristicDetect 按 rules 顺序找第一条满足的（命中 must 中任意一个即算命中）。
// 这是退化策略，准确率不如 chroma，但对短代码比 chroma.Analyse 命中率显著更高。
func heuristicDetect(src string) string {
	for _, r := range heuristicRules {
		for _, re := range r.must {
			if re.MatchString(src) {
				return r.name
			}
		}
	}
	return ""
}

// ---- 自然语言判别 ----------------------------------------------------------

// IsMostlyCJK 判断中文字符占比是否超过阈值（导出版，供 watcher 等上游复用）。
// 只关心中文（unicode.Han）；当前需求未涉及日文/韩文笔记的误判场景。
func IsMostlyCJK(s string) bool { return isMostlyCJK(s) }

// isMostlyCJK 内部实现（保留小写名以便包内继续按既有命名调用）。
func isMostlyCJK(s string) bool {
	var total, cjk int
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.Is(unicode.Han, r) {
			cjk++
		}
	}
	if total == 0 {
		return false
	}
	return float64(cjk)/float64(total) > cjkRatioThreshold
}

// looksLikeMarkdown 宽松规则：含 ``` 围栏、或行首出现 markdown 标记字符。
// 仅用于 CJK 主导场景的二次确认 —— 此时上下文已大概率是中文笔记，
// 命中单字符标记即可视作 markdown。
func looksLikeMarkdown(s string) bool {
	if strings.Contains(s, "```") {
		return true
	}
	for line := range strings.SplitSeq(s, "\n") {
		t := strings.TrimLeft(line, " \t")
		if t == "" {
			continue
		}
		switch t[0] {
		case '#', '>', '-', '*':
			return true
		}
	}
	return false
}

// markdown 强信号正则：命中其一即可一票通过判为 markdown。
//   - 代码围栏 ``` 在 looksLikeMarkdownStrong 内单独用 strings.Contains 判断
//   - 其它强信号必须有"括号 / 标签 / 配对星号"等语法结构，避免与代码片段冲突。
var (
	mdStrongImage    = regexp.MustCompile(`!\[[^\]]*\]\(`)             // ![alt](
	mdStrongLink     = regexp.MustCompile(`\[[^\]]+\]\([^)\s]+\)`)      // [text](url)
	mdStrongAutolink = regexp.MustCompile(`<https?://[^\s>]+>`)         // <https://...>
	mdStrongBold     = regexp.MustCompile(`\*\*[^*\s][^*]*[^*\s]\*\*`)  // **bold**（至少 2 字符内容）
)

// markdown 弱信号正则（行首匹配，需 ≥ 2 类同时命中才算）：
//   - 标题：必须是 # 后接空格，规避 C 的 #include / #define
//   - 引用：> 后接空格
//   - 列表：- / * / + / 数字. 后接空格
//   - 分隔线：---  单独一行
var (
	mdWeakHeading = regexp.MustCompile(`(?m)^#{1,6}\s+\S`)
	mdWeakQuote   = regexp.MustCompile(`(?m)^>\s+\S`)
	mdWeakList    = regexp.MustCompile(`(?m)^\s*(?:[-*+]\s+\S|\d+\.\s+\S)`)
	mdWeakHRule   = regexp.MustCompile(`(?m)^-{3,}\s*$`)
)

// looksLikeMarkdownStrong 强 markdown 检测：
//   - 命中任一强信号（代码围栏 / 图片 / 链接 / autolink / 成对粗体）即返回 true
//   - 否则要求 ≥ 2 类不同弱信号同时出现
//
// 设计目标：覆盖中英混合的 markdown 笔记（CJK 占比 < 30% 的场景），同时
// 避免把代码片段（C 的 #include、shell 的 ls -l 输出、YAML 列表项等）误判。
func looksLikeMarkdownStrong(s string) bool {
	// 代码围栏：成对出现的 ``` 几乎只在 markdown 里有
	if strings.Contains(s, "```") {
		return true
	}
	if mdStrongImage.MatchString(s) ||
		mdStrongLink.MatchString(s) ||
		mdStrongAutolink.MatchString(s) ||
		mdStrongBold.MatchString(s) {
		return true
	}
	// 弱信号组合：≥ 2 类
	hits := 0
	for _, re := range []*regexp.Regexp{mdWeakHeading, mdWeakQuote, mdWeakList, mdWeakHRule} {
		if re.MatchString(s) {
			hits++
			if hits >= 2 {
				return true
			}
		}
	}
	return false
}
