package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

// Calculator 是 L0 风险的内置工具：执行确定性数学表达式。
// 为了避免引入第三方表达式引擎，这里实现一个小型 shunting-yard 解析器，
// 支持 + - * / % ^、括号、一元负号，以及常见函数（sqrt / abs / log / ln / sin / cos / tan / exp）。
// 如果表达式包含未知标识符或语法错误则报错；不允许变量赋值。
type Calculator struct{}

func NewCalculator() *Calculator { return &Calculator{} }

var calculatorInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "expression": {
      "type": "string",
      "description": "纯数学表达式，例如 (1+2)*3、sqrt(16)、2^10、sin(3.14)。不支持变量。"
    }
  },
  "required": ["expression"],
  "additionalProperties": false
}`)

var calculatorOutputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "expression": { "type": "string" },
    "result": { "type": "number" }
  }
}`)

func (Calculator) Spec() Spec {
	return Spec{
		Name:         "calculator",
		Description:  "执行确定性数学表达式计算。支持 + - * / % ^、括号以及 sqrt/abs/log/ln/sin/cos/tan/exp 函数。",
		Category:     "utility",
		RiskLevel:    RiskL0,
		InputSchema:  calculatorInputSchema,
		OutputSchema: calculatorOutputSchema,
		TimeoutMS:    5000,
		ProviderType: "builtin",
	}
}

type calculatorArgs struct {
	Expression string `json:"expression"`
}

func (Calculator) Execute(_ context.Context, args json.RawMessage) (any, error) {
	var in calculatorArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	expr := strings.TrimSpace(in.Expression)
	if expr == "" {
		return nil, errors.New("calculator: empty expression")
	}
	result, err := evalExpression(expr)
	if err != nil {
		return nil, err
	}
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return nil, errors.New("calculator: result is NaN or Inf")
	}
	return map[string]any{
		"expression": expr,
		"result":     result,
	}, nil
}

// ========= 表达式解析器 =========

// tokenKind 是词法分析产出的词种。
type tokenKind int

const (
	tokNumber tokenKind = iota
	tokIdent
	tokOp
	tokLParen
	tokRParen
	tokComma
)

type token struct {
	kind tokenKind
	text string
	num  float64
}

// tokenize 扫描一遍表达式得到 token 序列。
// 为了让 `-1` 这种一元负号能正常工作，把它在这一层转换成 `0 - 1` 的形式。
func tokenize(s string) ([]token, error) {
	out := make([]token, 0, len(s))
	i := 0
	prevIsValue := false // 上一个 token 是不是数值/右括号/标识符——用于识别一元负号
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c >= '0' && c <= '9' || c == '.':
			j := i
			for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == '.') {
				j++
			}
			// 科学计数法 e/E
			if j < len(s) && (s[j] == 'e' || s[j] == 'E') {
				j++
				if j < len(s) && (s[j] == '+' || s[j] == '-') {
					j++
				}
				for j < len(s) && s[j] >= '0' && s[j] <= '9' {
					j++
				}
			}
			var v float64
			if _, err := fmt.Sscanf(s[i:j], "%g", &v); err != nil {
				return nil, fmt.Errorf("calculator: bad number %q", s[i:j])
			}
			out = append(out, token{kind: tokNumber, num: v})
			i = j
			prevIsValue = true
		case isLetter(c):
			j := i
			for j < len(s) && (isLetter(s[j]) || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			out = append(out, token{kind: tokIdent, text: s[i:j]})
			i = j
			prevIsValue = true
		case c == '(':
			out = append(out, token{kind: tokLParen, text: "("})
			i++
			prevIsValue = false
		case c == ')':
			out = append(out, token{kind: tokRParen, text: ")"})
			i++
			prevIsValue = true
		case c == ',':
			out = append(out, token{kind: tokComma, text: ","})
			i++
			prevIsValue = false
		case c == '+' || c == '-' || c == '*' || c == '/' || c == '%' || c == '^':
			if c == '-' && !prevIsValue {
				// 一元负号：转换成 0 - X
				out = append(out, token{kind: tokNumber, num: 0})
			}
			if c == '+' && !prevIsValue {
				// 一元正号：直接丢弃
				i++
				continue
			}
			out = append(out, token{kind: tokOp, text: string(c)})
			i++
			prevIsValue = false
		default:
			return nil, fmt.Errorf("calculator: unexpected char %q", string(c))
		}
	}
	return out, nil
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' }

// 运算符优先级与结合性。
func precedence(op string) int {
	switch op {
	case "+", "-":
		return 1
	case "*", "/", "%":
		return 2
	case "^":
		return 3
	}
	return 0
}
func rightAssoc(op string) bool { return op == "^" }

// evalExpression 走 shunting-yard 生成 RPN，再逐步求值。
func evalExpression(s string) (float64, error) {
	toks, err := tokenize(s)
	if err != nil {
		return 0, err
	}
	var (
		rpn       []token
		opStack   []token
		funcStack []token
	)
	for _, t := range toks {
		switch t.kind {
		case tokNumber:
			rpn = append(rpn, t)
		case tokIdent:
			funcStack = append(funcStack, t)
			opStack = append(opStack, t) // 函数按标识符压运算符栈
		case tokOp:
			for len(opStack) > 0 {
				top := opStack[len(opStack)-1]
				if top.kind != tokOp {
					break
				}
				if precedence(top.text) > precedence(t.text) ||
					(precedence(top.text) == precedence(t.text) && !rightAssoc(t.text)) {
					rpn = append(rpn, top)
					opStack = opStack[:len(opStack)-1]
					continue
				}
				break
			}
			opStack = append(opStack, t)
		case tokLParen:
			opStack = append(opStack, t)
		case tokComma:
			// MVP 只有一元函数，不应出现逗号
			return 0, errors.New("calculator: comma not supported")
		case tokRParen:
			found := false
			for len(opStack) > 0 {
				top := opStack[len(opStack)-1]
				opStack = opStack[:len(opStack)-1]
				if top.kind == tokLParen {
					found = true
					break
				}
				rpn = append(rpn, top)
			}
			if !found {
				return 0, errors.New("calculator: unbalanced parenthesis")
			}
			// 如果 ( 的前一个是函数标识符，弹出
			if len(opStack) > 0 && opStack[len(opStack)-1].kind == tokIdent {
				rpn = append(rpn, opStack[len(opStack)-1])
				opStack = opStack[:len(opStack)-1]
			}
		}
	}
	_ = funcStack
	for len(opStack) > 0 {
		top := opStack[len(opStack)-1]
		opStack = opStack[:len(opStack)-1]
		if top.kind == tokLParen {
			return 0, errors.New("calculator: unbalanced parenthesis")
		}
		rpn = append(rpn, top)
	}
	return evalRPN(rpn)
}

func evalRPN(rpn []token) (float64, error) {
	var stack []float64
	pop := func() (float64, error) {
		if len(stack) == 0 {
			return 0, errors.New("calculator: stack underflow")
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v, nil
	}
	for _, t := range rpn {
		switch t.kind {
		case tokNumber:
			stack = append(stack, t.num)
		case tokOp:
			b, err := pop()
			if err != nil {
				return 0, err
			}
			a, err := pop()
			if err != nil {
				return 0, err
			}
			switch t.text {
			case "+":
				stack = append(stack, a+b)
			case "-":
				stack = append(stack, a-b)
			case "*":
				stack = append(stack, a*b)
			case "/":
				if b == 0 {
					return 0, errors.New("calculator: division by zero")
				}
				stack = append(stack, a/b)
			case "%":
				if b == 0 {
					return 0, errors.New("calculator: modulo by zero")
				}
				stack = append(stack, math.Mod(a, b))
			case "^":
				stack = append(stack, math.Pow(a, b))
			default:
				return 0, fmt.Errorf("calculator: unknown operator %q", t.text)
			}
		case tokIdent:
			x, err := pop()
			if err != nil {
				return 0, err
			}
			v, err := applyFunc(t.text, x)
			if err != nil {
				return 0, err
			}
			stack = append(stack, v)
		default:
			return 0, fmt.Errorf("calculator: unexpected token kind %d", t.kind)
		}
	}
	if len(stack) != 1 {
		return 0, errors.New("calculator: invalid expression")
	}
	return stack[0], nil
}

// applyFunc 执行已知的一元函数。
func applyFunc(name string, x float64) (float64, error) {
	switch strings.ToLower(name) {
	case "sqrt":
		if x < 0 {
			return 0, errors.New("calculator: sqrt of negative")
		}
		return math.Sqrt(x), nil
	case "abs":
		return math.Abs(x), nil
	case "log":
		if x <= 0 {
			return 0, errors.New("calculator: log of non-positive")
		}
		return math.Log10(x), nil
	case "ln":
		if x <= 0 {
			return 0, errors.New("calculator: ln of non-positive")
		}
		return math.Log(x), nil
	case "sin":
		return math.Sin(x), nil
	case "cos":
		return math.Cos(x), nil
	case "tan":
		return math.Tan(x), nil
	case "exp":
		return math.Exp(x), nil
	case "pi":
		return math.Pi, nil
	case "e":
		return math.E, nil
	}
	return 0, fmt.Errorf("calculator: unknown function %q", name)
}
