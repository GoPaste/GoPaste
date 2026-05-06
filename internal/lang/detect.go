// Package lang 提供基于内容的源代码语言检测能力。
//
// 设计取舍：
//   - 第一道防线：强 markdown 检测（围栏 / 图片 / 链接 / autolink 一票通过，
//     或多类弱信号组合）。前置目的是覆盖"中英混合的 markdown 笔记"场景：
//     这类内容 CJK 占比常 < 30%，过去会落到代码识别链路被错判。
//   - 第二道防线：CJK 字符占比高 → 直接判 markdown（弱规则）/ 空，规避典型
//     误判（比如"中文说明 + 嵌入 git/mkdir 命令"被误识别为 Bash）。
//   - 第三道防线：手写正则启发式覆盖主流 10+ 语言。规则按特异性排序，先匹配的赢。
//   - 第四道防线：chroma.Analyse 内置 lexer 自评分兜底，但只放行主流语言白名单，
//     避免 Gdscript3 / Modula-2 等冷门 lexer 对日志/普通代码过度自信抢匹配。
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

	// 第零道：强代码特征前置。
	// markdown 强检测里的 ![](), [](), <http://...>, **bold** 等语法，
	// 可以合法地出现在源码的字符串/反引号字面量里（典型场景：lang 自己的
	// detect_test.go、README 生成脚本、爬虫里拼 markdown 的 Python 代码）。
	// 这类文件同时具备"代码三件套"和"markdown 强信号"，若让 markdown
	// 一票通过会把源码误判成笔记。所以先看是否有几乎不可能出现在 markdown
	// 笔记里的代码组合（如 Go 的 package+import+func 同时出现），命中就跳过
	// markdown 强检测，直接进入启发式代码识别。
	if looksLikeCodeStrong(sample) {
		if name := heuristicDetect(sample); name != "" {
			return name
		}
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
	// 只保留白名单内的常见语言——chroma 收录了 200+ lexer，很多冷门 lexer
	// （GDScript3 / Modula-2 / Genshi 等）会对普通代码/日志过度自信抢匹配，
	// 对用户而言这些标签基本无意义，不如退回通用 "Code"。
	if l := lexers.Analyse(sample); l != nil {
		name := normalizeChromaName(l.Config().Name)
		if isAllowedChromaLang(name) {
			return name
		}
	}

	return ""
}

// normalizeChromaName 把 chroma 的展示名（"Go" "C++" "JavaScript"）转成小写标准名。
func normalizeChromaName(name string) string {
	return strings.ToLower(name)
}

// chromaAllowlist 第四道防线放行的主流语言集合。
// 判据：语言流行度高、日常剪贴场景常见，或在启发式规则里未覆盖但识别价值明确。
// 不在表内的一律回落到通用 "Code" 标签，避免 Gdscript3 / Modula-2 之类无意义结果。
var chromaAllowlist = map[string]struct{}{
	"go":         {},
	"python":     {},
	"python 2":   {},
	"javascript": {},
	"typescript": {},
	"java":       {},
	"kotlin":     {},
	"scala":      {},
	"groovy":     {},
	"c":          {},
	"c++":        {},
	"c#":         {},
	"objective-c": {},
	"swift":      {},
	"rust":       {},
	"ruby":       {},
	"php":        {},
	"perl":       {},
	"lua":        {},
	"r":          {},
	"dart":       {},
	"elixir":     {},
	"erlang":     {},
	"haskell":    {},
	"ocaml":      {},
	"clojure":    {},
	"bash":       {},
	"shell":      {},
	"powershell": {},
	"sql":        {},
	"html":       {},
	"css":        {},
	"scss":       {},
	"less":       {},
	"xml":        {},
	"yaml":       {},
	"toml":       {},
	"json":       {},
	"ini":        {},
	"markdown":   {},
	"dockerfile": {},
	"makefile":   {},
	"nginx configuration file": {},
}

func isAllowedChromaLang(name string) bool {
	_, ok := chromaAllowlist[name]
	return ok
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
		// JSON：必须含完整的 "key": value 结构。
		// 历史上这里还有一条 `^\s*[{\[]`（仅要求以 { 或 [ 开头）的规则，
		// 但这个规则过于宽松——任何以方括号开头的文本（如 `[DIAG] ...`
		// 日志、`[INFO] ...` 命令行输出、中文笔记里的 `[备注]` 标签等）
		// 都会被误判成 JSON。删除后只保留带引号键的结构性规则：
		//   - 正面：{"name":"x"} / 格式化 JSON / 含对象元素的 JSON 数组 全部正常命中
		//   - 代价：纯字面量数组 [1,2,3] 不再被识别为 JSON，但这类内容在剪贴板
		//     里极少出现且会兜底到通用 Code 标签，权衡可接受
		{"json", []*regexp.Regexp{
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

// ---- 强代码特征（用于抢在 markdown 强检测之前放行）-----------------------

// codeStrongRule 一条强代码特征规则：只有 all 里的全部正则都命中才算。
// 与 heuristicRules 的"任一命中"不同，这里要求**同时**命中多条，
// 用来识别"几乎不可能是 markdown 笔记"的源码组合。
// 设计权衡：
//   - 命中阈值高 → 漏判率高（部分源码走不进来），但这没关系 —— 漏掉的
//     会继续走第一道 markdown / 第三道启发式，不会变差。
//   - 命中阈值高 → 误判率极低 → 可以安全地抢在 markdown 强检测之前。
type codeStrongRule struct {
	name string
	all  []*regexp.Regexp
}

var codeStrongRules = []codeStrongRule{
	// Go：package 声明 + import + func 同时出现。markdown 笔记里凑齐这三件
	// 极不现实（即便摘抄代码也通常只贴函数体片段）。
	{"go", []*regexp.Regexp{
		regexp.MustCompile(`(?m)^package\s+\w+\s*$`),
		regexp.MustCompile(`(?m)^import\s+["(]`),
		regexp.MustCompile(`\bfunc\s+(?:\(\w+\s+\*?\w+\)\s+)?\w+\s*\(`),
	}},
	// Rust：fn + let + impl/trait/use 同时出现。
	{"rust", []*regexp.Regexp{
		regexp.MustCompile(`\bfn\s+\w+\s*(?:<[^>]+>)?\s*\(`),
		regexp.MustCompile(`\blet\s+(?:mut\s+)?\w+\s*[:=]`),
		regexp.MustCompile(`\b(?:impl|trait|pub\s+fn|use\s+\w+::)`),
	}},
	// Java：package 语句 + public class + main 方法。
	{"java", []*regexp.Regexp{
		regexp.MustCompile(`(?m)^package\s+[\w.]+\s*;`),
		regexp.MustCompile(`\bpublic\s+(?:static\s+)?(?:final\s+)?class\s+\w+`),
		regexp.MustCompile(`\bpublic\s+static\s+void\s+main\s*\(`),
	}},
	// Python：def 函数定义 + 顶格 import/from 语句。两条都要命中，避免笔记里
	// 偶尔贴个 `def foo():` 就误判。
	{"python", []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*def\s+\w+\s*\(.*\)\s*:`),
		regexp.MustCompile(`(?m)^\s*(?:from\s+[\w.]+\s+import\s+|import\s+\w+\s*$)`),
	}},
}

// looksLikeCodeStrong 命中任一语言的强代码组合则返回 true。
// 存在目的：抢在 markdown 强检测之前判明"显然是源码"的样本，
// 避免源码里合法出现的 markdown 语法字面量把识别带偏。
func looksLikeCodeStrong(src string) bool {
	for _, r := range codeStrongRules {
		matched := true
		for _, re := range r.all {
			if !re.MatchString(src) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
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
