package soyjs

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/robfig/soy/parse"
)

var reservedWords = []string{
	"break", "case", "catch", "class", "const", "continue", "debugger", "default", "delete", "do",
	"else", "enum", "export", "extends", "false", "finally", "for", "function", "if",
	"implements", "import", "in", "instanceof", "interface", "let", "null", "new", "package",
	"private", "protected", "public", "return", "static", "super", "switch", "this", "throw",
	"true", "try", "typeof", "var", "void", "while", "with", "yield",
}

var reservedWordSet map[string]struct{}

func init() {
	reservedWordSet = make(map[string]struct{}, len(reservedWords))
	for _, word := range reservedWords {
		reservedWordSet[word] = struct{}{}
	}
}

type state struct {
	wr           io.Writer
	node         parse.Node // current node, for errors
	indentLevels int
	namespace    string
}

func Write(out io.Writer, node parse.Node) (err error) {
	defer errRecover(&err)
	(&state{wr: out}).walk(node)
	return nil
}

// at marks the state to be on node n, for error reporting.
func (s *state) at(node parse.Node) {
	s.node = node
}

// errorf formats the error and terminates processing.
func (s *state) errorf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

// errRecover is the handler that turns panics into returns from the top
// level of Parse.
func errRecover(errp *error) {
	e := recover()
	if e != nil {
		*errp = fmt.Errorf("%v", e)
	}
}

// walk recursively goes through each node and executes the indicated logic and
// writes the output
func (s *state) walk(node parse.Node) {
	s.at(node)
	switch node := node.(type) {
	case *parse.SoyFileNode:
		s.visitSoyFile(node)
	case *parse.NamespaceNode:
		s.visitNamespace(node)
	case *parse.SoyDocNode:
		return
	case *parse.TemplateNode:
		s.visitTemplate(node)
	case *parse.ListNode:
		s.visitChildren(node)

		// Output nodes ----------
	case *parse.RawTextNode:
		s.jsln("output += ", strconv.Quote(string(node.Text)), ";")
	case *parse.PrintNode:
		// TODO: Directives
		s.indent()
		s.js("output += ")
		s.visitChildren(node)
		s.js(";\n")

	// case *parse.MsgNode:
	// 	s.walk(node.Body)
	// case *parse.CssNode:
	// 	var prefix = ""
	// 	if node.Expr != nil {
	// 		prefix = s.eval(node.Expr).String() + "-"
	// 	}
	// 	s.wr.Write([]byte(prefix + node.Suffix))
	case *parse.DebuggerNode:
		s.jsln("debugger;")
	// case *parse.LogNode:
	// 	s.jsln(fmt.Sprintf("console.log(%q);\n", s.renderBlock(node.Body)))

	// Control flow ----------
	case *parse.IfNode:
		s.indent()
		for i, branch := range node.Conds {
			if i > 0 {
				s.js("else ")
			}
			if branch.Cond != nil {
				s.js("if (")
				s.walk(branch.Cond)
			}
			s.indentLevels++
			s.indentLevels--
			s.js(") {\n")
		}

	case *parse.ForNode:
		s.visitFor(node)
	// case *parse.SwitchNode:
	// 	var switchValue = s.eval(node.Value)
	// 	for _, caseNode := range node.Cases {
	// 		for _, caseValueNode := range caseNode.Values {
	// 			if switchValue.Equals(s.eval(caseValueNode)) {
	// 				s.walk(caseNode.Body)
	// 				return
	// 			}
	// 		}
	// 		if len(caseNode.Values) == 0 { // default/last case
	// 			s.walk(caseNode.Body)
	// 			return
	// 		}
	// 	}

	case *parse.CallNode:
		s.visitCall(node)
	// case *parse.LetValueNode:
	// 	s.newln()
	// 	s.js("var ", node.Name, " = ")
	// 	s.walk(node.Expr)
	// 	s.js(";\n")
	// case *parse.LetContentNode:
	// 	s.context.set(node.Name, data.String(s.renderBlock(node.Body)))

	// Values ----------
	case *parse.NullNode:
		s.js("null")
	case *parse.StringNode:
		s.js(strconv.Quote(node.Value))
	case *parse.IntNode:
		s.js(node.String())
	case *parse.FloatNode:
		s.js(node.String())
	case *parse.BoolNode:
		s.js(node.String())
	case *parse.GlobalNode:
		s.js(node.Name)
	case *parse.ListLiteralNode:
		s.js("[")
		for i, item := range node.Items {
			if i != 0 {
				s.js(",")
			}
			s.walk(item)
		}
		s.js("]")
	case *parse.MapLiteralNode:
		s.js("{")
		var first = true
		for k, v := range node.Items {
			if !first {
				s.js(",")
			}
			first = false
			s.js(k, ":")
			s.walk(v)
		}
		s.js("}")
	case *parse.FunctionNode:
		s.js(node.Name, "(")
		for i, arg := range node.Args {
			if i != 0 {
				s.js(",")
			}
			s.walk(arg)
		}
		s.js(")")
	case *parse.DataRefNode:
		s.visitDataRef(node)

	// 	// Arithmetic operators ----------
	case *parse.NegateNode:
		s.js("(-")
		s.walk(node.Arg)
		s.js(")")
	case *parse.AddNode:
		s.op("+", node)
	case *parse.SubNode:
		s.op("-", node)
	case *parse.DivNode:
		s.op("/", node)
	case *parse.MulNode:
		s.op("*", node)
	case *parse.ModNode:
		s.op("%", node)

		// Arithmetic comparisons ----------
	case *parse.EqNode:
		s.op("==", node)
	case *parse.NotEqNode:
		s.op("!=", node)
	case *parse.LtNode:
		s.op("<", node)
	case *parse.LteNode:
		s.op("<=", node)
	case *parse.GtNode:
		s.op(">", node)
	case *parse.GteNode:
		s.op(">=", node)

	// Boolean operators ----------
	case *parse.NotNode:
		s.js("!(")
		s.walk(node.Arg)
		s.js(")")
	case *parse.AndNode:
		s.op("&&", node)
	case *parse.OrNode:
		s.op("||", node)
	case *parse.ElvisNode:
		s.op("||", node)
	case *parse.TernNode:
		s.js("(")
		s.walk(node.Arg1)
		s.js("?")
		s.walk(node.Arg2)
		s.js(":")
		s.walk(node.Arg3)
		s.js(")")

	default:
		s.errorf("unknown node (%T): %v", node, node)
	}
}

func (s *state) visitSoyFile(node *parse.SoyFileNode) {
	s.jsln("// This file was automatically generated from ", node.Name, ".")
	s.jsln("// Please don't edit this file by hand.")
	s.jsln("")
	s.visitChildren(node)
}

func (s *state) visitChildren(parent parse.ParentNode) {
	for _, child := range parent.Children() {
		s.walk(child)
	}
}

func (s *state) visitNamespace(node *parse.NamespaceNode) {
	s.namespace = node.Name

	// iterate through the dot segments.
	var i = 0
	for i < len(node.Name) {
		prev := i + 1
		i = strings.Index(node.Name[prev:], ".")
		if i == -1 {
			i = len(node.Name)
		} else {
			i += prev
		}
		s.jsln("if (typeof ", node.Name[:i], " == 'undefined') { var ", node.Name[:i], " = {}; }")
	}
}

func (s *state) visitTemplate(node *parse.TemplateNode) {
	s.jsln("")
	s.jsln("")
	s.jsln(node.Name, " = function(opt_data, opt_sb, opt_ijData) {")
	s.indentLevels++
	s.jsln("var output = '';")
	s.walk(node.Body)
	s.jsln("return output;")
	s.indentLevels--
	s.jsln("};")
}

func (s *state) visitDataRef(node *parse.DataRefNode) {
	if node.Key == "ij" {
		s.js("opt_ijData")
	} else {
		s.js("opt_data.", node.Key)
	}
	for _, accessNode := range node.Access {
		switch node := accessNode.(type) {
		case *parse.DataRefIndexNode:
			s.js("[", strconv.Itoa(node.Index), "]")
		case *parse.DataRefKeyNode:
			s.js(".", node.Key)
		case *parse.DataRefExprNode:
			s.js("[")
			s.walk(node.Arg)
			s.js("]")
		}
	}
}

func (s *state) visitCall(node *parse.CallNode) {
	var dataExpr = "opt_data"
	if node.Data != nil {
		dataExpr = s.str(node.Data)
	}
	if len(node.Params) > 0 {
		dataExpr = "soy.$$augmentMap(" + dataExpr + ", {"
		for _, param := range node.Params {
			switch param := param.(type) {
			case *parse.CallParamValueNode:
				// TODO: Reference to data item.
				dataExpr += param.Key + ": "
				s.walk()
			case *parse.CallParamContentNode:
				t.errorf("unimplemented")
			}
		}
	}
	s.jsln("output += ", node.Name, "(", dataExpr, ", undefined, opt_ijData);")
}

func (s *state) visitFor(node *parse.ForNode) {
	var foreachList, isForeach = node.List.(*parse.DataRefNode)
	if !isForeach {
		s.errorf("for range() unimplemented")
	}

	// TODO: uniquify the variable names
	var (
		itemList    = node.Var + "List"
		itemListLen = node.Var + "Len"
		itemData    = node.Var + "Data"
		itemIndex   = node.Var + "Index"
	)
	s.indent()
	s.js("var ", itemList, " = ")
	s.walk(foreachList)
	s.js(";\n")
	s.jsln("var ", itemListLen, " = ", itemList, ".length;")
	s.jsln("if (", itemListLen, " > 0) {")
	s.indentLevels++
	s.jsln("for (var ", itemIndex, " = 0; ", itemIndex, " < ", itemListLen, "; ", itemIndex, "++) {")
	s.indentLevels++
	s.jsln("var ", itemData, " = ", itemList, "[", itemIndex, "];")
	s.walk(node.Body)
	s.jsln("}")
	s.jsln("} else {")
	// TODO: Omit the if/else if there is no {ifempty}
	if node.IfEmpty != nil {
		s.walk(node.IfEmpty)
	}
	s.jsln("}")
}

func (s *state) op(symbol string, node parse.ParentNode) {
	var children = node.Children()
	s.js("(")
	s.walk(children[0])
	s.js(" ", symbol, " ")
	s.walk(children[1])
	s.js(")")
}

func (s *state) js(args ...string) {
	for _, arg := range args {
		s.wr.Write([]byte(arg))
	}
}

func (s *state) indent() {
	for i := 0; i < s.indentLevels; i++ {
		s.wr.Write([]byte("  "))
	}
}

func (s *state) jsln(args ...string) {
	s.indent()
	for _, arg := range args {
		s.wr.Write([]byte(arg))
	}
	s.wr.Write([]byte("\n"))
}

var unescapes = map[rune]rune{
	'\\': '\\',
	'\'': '\'',
	'n':  '\n',
	'r':  '\r',
	't':  '\t',
	'b':  '\b',
	'f':  '\f',
}

var escapes = make(map[rune]rune)

func init() {
	for k, v := range unescapes {
		escapes[v] = k
	}
}

// TODO: hijacked quoteString from parse
func quoteString(s string) string {
	var q = make([]rune, 1, len(s)+10)
	q[0] = '\''
	for _, ch := range s {
		if seq, ok := escapes[ch]; ok {
			q = append(q, '\\', seq)
			continue
		}
		q = append(q, ch)
	}
	return string(append(q, '\''))
}