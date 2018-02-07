//line build/parse.y:13
package build

import __yyfmt__ "fmt"

//line build/parse.y:13
//line build/parse.y:18
type yySymType struct {
	yys int
	// input tokens
	tok    string   // raw input syntax
	str    string   // decoding of quoted string
	pos    Position // position of token
	triple bool     // was string triple quoted?

	// partial syntax trees
	expr    Expr
	exprs   []Expr
	forc    *ForClause
	ifs     []*IfClause
	forifs  *ForClauseWithIfClausesOpt
	forsifs []*ForClauseWithIfClausesOpt
	string  *StringExpr
	strings []*StringExpr
	block   CodeBlock

	// supporting information
	comma    Position // position of trailing comma in list, if present
	lastRule Expr     // most recent rule, to attach line comments to
}

const _ADDEQ = 57346
const _AND = 57347
const _COMMENT = 57348
const _EOF = 57349
const _EQ = 57350
const _FOR = 57351
const _GE = 57352
const _IDENT = 57353
const _IF = 57354
const _ELSE = 57355
const _ELIF = 57356
const _IN = 57357
const _IS = 57358
const _LAMBDA = 57359
const _LE = 57360
const _NE = 57361
const _NOT = 57362
const _OR = 57363
const _PYTHON = 57364
const _STRING = 57365
const _DEF = 57366
const _RETURN = 57367
const _INDENT = 57368
const _UNINDENT = 57369
const ShiftInstead = 57370
const _ASSERT = 57371
const _UNARY = 57372

var yyToknames = [...]string{
	"$end",
	"error",
	"$unk",
	"'%'",
	"'('",
	"')'",
	"'*'",
	"'+'",
	"','",
	"'-'",
	"'.'",
	"'/'",
	"':'",
	"'<'",
	"'='",
	"'>'",
	"'['",
	"']'",
	"'{'",
	"'}'",
	"_ADDEQ",
	"_AND",
	"_COMMENT",
	"_EOF",
	"_EQ",
	"_FOR",
	"_GE",
	"_IDENT",
	"_IF",
	"_ELSE",
	"_ELIF",
	"_IN",
	"_IS",
	"_LAMBDA",
	"_LE",
	"_NE",
	"_NOT",
	"_OR",
	"_PYTHON",
	"_STRING",
	"_DEF",
	"_RETURN",
	"_INDENT",
	"_UNINDENT",
	"ShiftInstead",
	"'\\n'",
	"_ASSERT",
	"_UNARY",
	"';'",
}
var yyStatenames = [...]string{}

const yyEofCode = 1
const yyErrCode = 2
const yyInitialStackSize = 16

//line build/parse.y:726

// Go helper code.

// unary returns a unary expression with the given
// position, operator, and subexpression.
func unary(pos Position, op string, x Expr) Expr {
	return &UnaryExpr{
		OpStart: pos,
		Op:      op,
		X:       x,
	}
}

// binary returns a binary expression with the given
// operands, position, and operator.
func binary(x Expr, pos Position, op string, y Expr) Expr {
	_, xend := x.Span()
	ystart, _ := y.Span()
	return &BinaryExpr{
		X:         x,
		OpStart:   pos,
		Op:        op,
		LineBreak: xend.Line < ystart.Line,
		Y:         y,
	}
}

// isSimpleExpression returns whether an expression is simple and allowed to exist in
// compact forms of sequences.
// The formal criteria are the following: an expression is considered simple if it's
// a literal (variable, string or a number), a literal with a unary operator or an empty sequence.
func isSimpleExpression(expr *Expr) bool {
	switch x := (*expr).(type) {
	case *LiteralExpr, *StringExpr:
		return true
	case *UnaryExpr:
		_, ok := x.X.(*LiteralExpr)
		return ok
	case *ListExpr:
		return len(x.List) == 0
	case *TupleExpr:
		return len(x.List) == 0
	case *DictExpr:
		return len(x.List) == 0
	case *SetExpr:
		return len(x.List) == 0
	default:
		return false
	}
}

// forceCompact returns the setting for the ForceCompact field for a call or tuple.
//
// NOTE 1: The field is called ForceCompact, not ForceSingleLine,
// because it only affects the formatting associated with the call or tuple syntax,
// not the formatting of the arguments. For example:
//
//	call([
//		1,
//		2,
//		3,
//	])
//
// is still a compact call even though it runs on multiple lines.
//
// In contrast the multiline form puts a linebreak after the (.
//
//	call(
//		[
//			1,
//			2,
//			3,
//		],
//	)
//
// NOTE 2: Because of NOTE 1, we cannot use start and end on the
// same line as a signal for compact mode: the formatting of an
// embedded list might move the end to a different line, which would
// then look different on rereading and cause buildifier not to be
// idempotent. Instead, we have to look at properties guaranteed
// to be preserved by the reformatting, namely that the opening
// paren and the first expression are on the same line and that
// each subsequent expression begins on the same line as the last
// one ended (no line breaks after comma).
func forceCompact(start Position, list []Expr, end Position) bool {
	if len(list) <= 1 {
		// The call or tuple will probably be compact anyway; don't force it.
		return false
	}

	// If there are any named arguments or non-string, non-literal
	// arguments, cannot force compact mode.
	line := start.Line
	for _, x := range list {
		start, end := x.Span()
		if start.Line != line {
			return false
		}
		line = end.Line
		if !isSimpleExpression(&x) {
			return false
		}
	}
	return end.Line == line
}

// forceMultiLine returns the setting for the ForceMultiLine field.
func forceMultiLine(start Position, list []Expr, end Position) bool {
	if len(list) > 1 {
		// The call will be multiline anyway, because it has multiple elements. Don't force it.
		return false
	}

	if len(list) == 0 {
		// Empty list: use position of brackets.
		return start.Line != end.Line
	}

	// Single-element list.
	// Check whether opening bracket is on different line than beginning of
	// element, or closing bracket is on different line than end of element.
	elemStart, elemEnd := list[0].Span()
	return start.Line != elemStart.Line || end.Line != elemEnd.Line
}

//line yacctab:1
var yyExca = [...]int{
	-1, 1,
	1, -1,
	-2, 0,
}

const yyNprod = 91
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 694

var yyAct = [...]int{

	13, 110, 133, 2, 135, 72, 17, 7, 115, 67,
	33, 9, 114, 79, 127, 57, 30, 58, 34, 63,
	64, 65, 157, 29, 68, 70, 75, 100, 36, 37,
	82, 162, 106, 77, 71, 74, 83, 82, 32, 86,
	87, 88, 89, 90, 91, 92, 93, 94, 95, 96,
	97, 98, 99, 163, 101, 102, 103, 104, 84, 159,
	81, 108, 109, 24, 24, 20, 20, 150, 26, 26,
	107, 28, 145, 117, 85, 23, 23, 25, 25, 117,
	117, 63, 130, 120, 124, 122, 27, 27, 117, 131,
	129, 128, 18, 18, 66, 19, 19, 15, 29, 29,
	14, 136, 24, 123, 134, 174, 168, 26, 138, 113,
	149, 167, 143, 144, 23, 69, 25, 126, 112, 144,
	164, 140, 111, 146, 34, 27, 151, 153, 148, 146,
	117, 146, 152, 60, 62, 156, 142, 29, 158, 59,
	118, 154, 139, 161, 160, 61, 121, 39, 78, 146,
	38, 41, 80, 42, 24, 40, 20, 39, 165, 26,
	38, 166, 35, 169, 170, 40, 23, 171, 25, 161,
	173, 7, 6, 1, 22, 11, 76, 27, 16, 73,
	24, 31, 20, 18, 12, 26, 19, 8, 15, 29,
	10, 14, 23, 172, 25, 5, 4, 147, 6, 3,
	21, 11, 116, 27, 16, 119, 0, 0, 0, 18,
	0, 0, 19, 0, 15, 29, 10, 14, 0, 39,
	0, 5, 38, 41, 0, 42, 0, 40, 125, 43,
	49, 44, 0, 0, 0, 0, 50, 54, 0, 0,
	45, 0, 48, 0, 56, 0, 0, 51, 55, 0,
	46, 47, 52, 53, 39, 0, 0, 38, 41, 0,
	42, 0, 40, 155, 43, 49, 44, 0, 0, 0,
	0, 50, 54, 0, 0, 45, 0, 48, 0, 56,
	0, 0, 51, 55, 0, 46, 47, 52, 53, 39,
	0, 0, 38, 41, 0, 42, 0, 40, 0, 43,
	49, 44, 0, 141, 0, 0, 50, 54, 0, 0,
	45, 0, 48, 0, 56, 0, 0, 51, 55, 0,
	46, 47, 52, 53, 39, 0, 0, 38, 41, 0,
	42, 0, 40, 0, 43, 49, 44, 0, 0, 0,
	0, 50, 54, 0, 0, 45, 117, 48, 0, 56,
	0, 0, 51, 55, 0, 46, 47, 52, 53, 39,
	0, 0, 38, 41, 0, 42, 0, 40, 0, 43,
	49, 44, 0, 0, 0, 0, 50, 54, 0, 0,
	45, 0, 48, 0, 56, 137, 0, 51, 55, 0,
	46, 47, 52, 53, 39, 0, 0, 38, 41, 0,
	42, 0, 40, 132, 43, 49, 44, 0, 0, 0,
	0, 50, 54, 0, 0, 45, 0, 48, 0, 56,
	0, 0, 51, 55, 0, 46, 47, 52, 53, 39,
	0, 0, 38, 41, 0, 42, 0, 40, 105, 43,
	49, 44, 0, 0, 0, 0, 50, 54, 0, 0,
	45, 0, 48, 0, 56, 0, 0, 51, 55, 0,
	46, 47, 52, 53, 39, 0, 0, 38, 41, 0,
	42, 0, 40, 0, 43, 49, 44, 0, 0, 0,
	0, 50, 54, 0, 0, 45, 0, 48, 0, 56,
	0, 0, 51, 55, 0, 46, 47, 52, 53, 39,
	0, 0, 38, 41, 0, 42, 0, 40, 0, 43,
	49, 44, 0, 0, 0, 0, 50, 54, 0, 0,
	45, 0, 48, 0, 0, 0, 0, 51, 55, 0,
	46, 47, 52, 53, 39, 0, 0, 38, 41, 0,
	42, 0, 40, 0, 43, 0, 44, 0, 0, 0,
	0, 0, 54, 0, 0, 45, 0, 48, 0, 56,
	0, 0, 51, 55, 0, 46, 47, 52, 53, 39,
	0, 0, 38, 41, 0, 42, 0, 40, 0, 43,
	0, 44, 0, 0, 0, 0, 0, 54, 0, 0,
	45, 0, 48, 0, 24, 0, 20, 51, 55, 26,
	46, 47, 52, 53, 0, 0, 23, 0, 25, 0,
	0, 0, 39, 0, 0, 38, 41, 27, 42, 0,
	40, 0, 43, 18, 44, 0, 19, 0, 15, 29,
	54, 14, 0, 45, 0, 48, 0, 0, 0, 0,
	0, 0, 0, 46, 47, 39, 53, 0, 38, 41,
	0, 42, 0, 40, 0, 43, 0, 44, 0, 0,
	0, 39, 0, 54, 38, 41, 45, 42, 48, 40,
	0, 43, 0, 44, 0, 0, 46, 47, 0, 0,
	0, 0, 45, 0, 48, 0, 0, 0, 0, 0,
	0, 0, 46, 47,
}
var yyPact = [...]int{

	-1000, -1000, 175, -1000, -1000, -1000, -30, -1000, -1000, -1000,
	10, 97, -2, 460, 59, -1000, 59, 128, 59, 59,
	59, -1000, -17, 59, 59, 59, 97, -1000, -1000, -1000,
	-1000, -36, 147, 28, 128, 59, 45, -1000, 59, 59,
	59, 59, 59, 59, 59, 59, 59, 59, 59, 59,
	59, 59, -5, 59, 59, 59, 59, 460, 425, 4,
	59, 59, 109, 460, -1000, -1000, -1000, 91, 320, 131,
	320, 140, 62, 83, 64, 215, 108, -1000, -32, 589,
	59, 59, 97, 390, 58, -1000, -1000, -1000, -1000, 153,
	153, 143, 143, 143, 143, 143, 143, 530, 530, 608,
	59, 641, 657, 608, 355, 58, -1000, 136, 320, 285,
	123, 59, 59, -1000, 54, -1000, -1000, 97, 59, -1000,
	104, -1000, 47, -1000, -1000, 59, 59, -1000, -1000, 135,
	250, 128, 58, -1000, -21, -1000, 608, 59, -1000, -1000,
	53, -1000, 59, 565, 460, -1000, -1000, 2, 21, -1000,
	-1000, 460, -1000, 215, 107, 58, -1000, -1000, 565, -1000,
	93, 460, 59, 59, 58, -1000, 149, -1000, 59, 495,
	495, -1000, -1000, 87, -1000,
}
var yyPgo = [...]int{

	0, 205, 0, 1, 6, 115, 9, 10, 202, 8,
	12, 200, 197, 3, 196, 187, 184, 4, 11, 181,
	5, 179, 176, 71, 174, 2, 173, 162, 148,
}
var yyR1 = [...]int{

	0, 26, 25, 25, 13, 13, 13, 13, 14, 14,
	15, 15, 15, 16, 16, 16, 27, 27, 17, 19,
	19, 18, 18, 18, 18, 28, 28, 4, 4, 4,
	4, 4, 4, 4, 4, 4, 4, 4, 4, 4,
	4, 4, 4, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 3, 3, 1, 1,
	20, 22, 22, 21, 21, 5, 5, 6, 6, 7,
	7, 23, 24, 24, 11, 8, 9, 10, 10, 12,
	12,
}
var yyR2 = [...]int{

	0, 2, 4, 1, 0, 2, 2, 3, 1, 1,
	7, 6, 1, 4, 5, 4, 2, 1, 4, 0,
	3, 1, 2, 1, 1, 0, 1, 1, 3, 4,
	4, 6, 8, 5, 1, 3, 4, 4, 4, 3,
	3, 3, 2, 1, 4, 2, 2, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 4, 3, 3, 3, 5, 0, 1, 0, 1,
	3, 1, 3, 1, 2, 1, 3, 0, 2, 1,
	3, 1, 1, 2, 1, 4, 2, 1, 2, 0,
	3,
}
var yyChk = [...]int{

	-1000, -26, -13, 24, -14, 46, 23, -17, -15, -18,
	41, 26, -16, -2, 42, 39, 29, -4, 34, 37,
	7, -11, -24, 17, 5, 19, 10, 28, -23, 40,
	46, -19, 28, -7, -4, -27, 30, 31, 7, 4,
	12, 8, 10, 14, 16, 25, 35, 36, 27, 15,
	21, 32, 37, 38, 22, 33, 29, -2, -2, 11,
	5, 17, -5, -2, -2, -2, -23, -6, -2, -5,
	-2, -6, -20, -21, -6, -2, -22, -4, -28, 49,
	5, 32, 9, -2, 13, 29, -2, -2, -2, -2,
	-2, -2, -2, -2, -2, -2, -2, -2, -2, -2,
	32, -2, -2, -2, -2, 13, 28, -6, -2, -2,
	-3, 13, 9, 18, -10, -9, -8, 26, 9, -1,
	-10, 6, -10, 20, 20, 13, 9, 46, -18, -6,
	-2, -4, 13, -25, 46, -17, -2, 30, -25, 6,
	-10, 18, 13, -2, -2, 18, -9, -12, -7, 6,
	20, -2, -20, -2, 6, 13, -25, 43, -2, 6,
	-3, -2, 29, 32, 13, -25, -13, 18, 13, -2,
	-2, -25, 44, -3, 18,
}
var yyDef = [...]int{

	4, -2, 0, 1, 5, 6, 0, 8, 9, 19,
	0, 0, 12, 21, 23, 24, 0, 43, 0, 0,
	0, 27, 34, 77, 77, 77, 0, 84, 82, 81,
	7, 25, 0, 0, 79, 0, 0, 17, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 22, 0, 0,
	77, 66, 0, 75, 45, 46, 83, 0, 75, 68,
	75, 0, 71, 0, 0, 75, 73, 42, 0, 26,
	77, 0, 0, 0, 0, 16, 47, 48, 49, 50,
	51, 52, 53, 54, 55, 56, 57, 58, 59, 60,
	0, 62, 63, 64, 0, 0, 28, 0, 75, 67,
	0, 0, 0, 35, 0, 87, 89, 0, 69, 78,
	0, 41, 0, 39, 40, 0, 74, 18, 20, 0,
	0, 80, 0, 15, 0, 3, 61, 0, 13, 29,
	0, 30, 66, 44, 76, 36, 88, 86, 0, 37,
	38, 70, 72, 0, 0, 0, 14, 4, 65, 33,
	0, 67, 0, 0, 0, 11, 0, 31, 66, 90,
	85, 10, 2, 0, 32,
}
var yyTok1 = [...]int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	46, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 4, 3, 3,
	5, 6, 7, 8, 9, 10, 11, 12, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 13, 49,
	14, 15, 16, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 17, 3, 18, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 19, 3, 20,
}
var yyTok2 = [...]int{

	2, 3, 21, 22, 23, 24, 25, 26, 27, 28,
	29, 30, 31, 32, 33, 34, 35, 36, 37, 38,
	39, 40, 41, 42, 43, 44, 45, 47, 48,
}
var yyTok3 = [...]int{
	0,
}

var yyErrorMessages = [...]struct {
	state int
	token int
	msg   string
}{}

//line yaccpar:1

/*	parser for yacc output	*/

var (
	yyDebug        = 0
	yyErrorVerbose = false
)

type yyLexer interface {
	Lex(lval *yySymType) int
	Error(s string)
}

type yyParser interface {
	Parse(yyLexer) int
	Lookahead() int
}

type yyParserImpl struct {
	lval  yySymType
	stack [yyInitialStackSize]yySymType
	char  int
}

func (p *yyParserImpl) Lookahead() int {
	return p.char
}

func yyNewParser() yyParser {
	return &yyParserImpl{}
}

const yyFlag = -1000

func yyTokname(c int) string {
	if c >= 1 && c-1 < len(yyToknames) {
		if yyToknames[c-1] != "" {
			return yyToknames[c-1]
		}
	}
	return __yyfmt__.Sprintf("tok-%v", c)
}

func yyStatname(s int) string {
	if s >= 0 && s < len(yyStatenames) {
		if yyStatenames[s] != "" {
			return yyStatenames[s]
		}
	}
	return __yyfmt__.Sprintf("state-%v", s)
}

func yyErrorMessage(state, lookAhead int) string {
	const TOKSTART = 4

	if !yyErrorVerbose {
		return "syntax error"
	}

	for _, e := range yyErrorMessages {
		if e.state == state && e.token == lookAhead {
			return "syntax error: " + e.msg
		}
	}

	res := "syntax error: unexpected " + yyTokname(lookAhead)

	// To match Bison, suggest at most four expected tokens.
	expected := make([]int, 0, 4)

	// Look for shiftable tokens.
	base := yyPact[state]
	for tok := TOKSTART; tok-1 < len(yyToknames); tok++ {
		if n := base + tok; n >= 0 && n < yyLast && yyChk[yyAct[n]] == tok {
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}
	}

	if yyDef[state] == -2 {
		i := 0
		for yyExca[i] != -1 || yyExca[i+1] != state {
			i += 2
		}

		// Look for tokens that we accept or reduce.
		for i += 2; yyExca[i] >= 0; i += 2 {
			tok := yyExca[i]
			if tok < TOKSTART || yyExca[i+1] == 0 {
				continue
			}
			if len(expected) == cap(expected) {
				return res
			}
			expected = append(expected, tok)
		}

		// If the default action is to accept or reduce, give up.
		if yyExca[i+1] != 0 {
			return res
		}
	}

	for i, tok := range expected {
		if i == 0 {
			res += ", expecting "
		} else {
			res += " or "
		}
		res += yyTokname(tok)
	}
	return res
}

func yylex1(lex yyLexer, lval *yySymType) (char, token int) {
	token = 0
	char = lex.Lex(lval)
	if char <= 0 {
		token = yyTok1[0]
		goto out
	}
	if char < len(yyTok1) {
		token = yyTok1[char]
		goto out
	}
	if char >= yyPrivate {
		if char < yyPrivate+len(yyTok2) {
			token = yyTok2[char-yyPrivate]
			goto out
		}
	}
	for i := 0; i < len(yyTok3); i += 2 {
		token = yyTok3[i+0]
		if token == char {
			token = yyTok3[i+1]
			goto out
		}
	}

out:
	if token == 0 {
		token = yyTok2[1] /* unknown char */
	}
	if yyDebug >= 3 {
		__yyfmt__.Printf("lex %s(%d)\n", yyTokname(token), uint(char))
	}
	return char, token
}

func yyParse(yylex yyLexer) int {
	return yyNewParser().Parse(yylex)
}

func (yyrcvr *yyParserImpl) Parse(yylex yyLexer) int {
	var yyn int
	var yyVAL yySymType
	var yyDollar []yySymType
	_ = yyDollar // silence set and not used
	yyS := yyrcvr.stack[:]

	Nerrs := 0   /* number of errors */
	Errflag := 0 /* error recovery flag */
	yystate := 0
	yyrcvr.char = -1
	yytoken := -1 // yyrcvr.char translated into internal numbering
	defer func() {
		// Make sure we report no lookahead when not parsing.
		yystate = -1
		yyrcvr.char = -1
		yytoken = -1
	}()
	yyp := -1
	goto yystack

ret0:
	return 0

ret1:
	return 1

yystack:
	/* put a state and value onto the stack */
	if yyDebug >= 4 {
		__yyfmt__.Printf("char %v in %v\n", yyTokname(yytoken), yyStatname(yystate))
	}

	yyp++
	if yyp >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyS[yyp] = yyVAL
	yyS[yyp].yys = yystate

yynewstate:
	yyn = yyPact[yystate]
	if yyn <= yyFlag {
		goto yydefault /* simple state */
	}
	if yyrcvr.char < 0 {
		yyrcvr.char, yytoken = yylex1(yylex, &yyrcvr.lval)
	}
	yyn += yytoken
	if yyn < 0 || yyn >= yyLast {
		goto yydefault
	}
	yyn = yyAct[yyn]
	if yyChk[yyn] == yytoken { /* valid shift */
		yyrcvr.char = -1
		yytoken = -1
		yyVAL = yyrcvr.lval
		yystate = yyn
		if Errflag > 0 {
			Errflag--
		}
		goto yystack
	}

yydefault:
	/* default state action */
	yyn = yyDef[yystate]
	if yyn == -2 {
		if yyrcvr.char < 0 {
			yyrcvr.char, yytoken = yylex1(yylex, &yyrcvr.lval)
		}

		/* look through exception table */
		xi := 0
		for {
			if yyExca[xi+0] == -1 && yyExca[xi+1] == yystate {
				break
			}
			xi += 2
		}
		for xi += 2; ; xi += 2 {
			yyn = yyExca[xi+0]
			if yyn < 0 || yyn == yytoken {
				break
			}
		}
		yyn = yyExca[xi+1]
		if yyn < 0 {
			goto ret0
		}
	}
	if yyn == 0 {
		/* error ... attempt to resume parsing */
		switch Errflag {
		case 0: /* brand new error */
			yylex.Error(yyErrorMessage(yystate, yytoken))
			Nerrs++
			if yyDebug >= 1 {
				__yyfmt__.Printf("%s", yyStatname(yystate))
				__yyfmt__.Printf(" saw %s\n", yyTokname(yytoken))
			}
			fallthrough

		case 1, 2: /* incompletely recovered error ... try again */
			Errflag = 3

			/* find a state where "error" is a legal shift action */
			for yyp >= 0 {
				yyn = yyPact[yyS[yyp].yys] + yyErrCode
				if yyn >= 0 && yyn < yyLast {
					yystate = yyAct[yyn] /* simulate a shift of "error" */
					if yyChk[yystate] == yyErrCode {
						goto yystack
					}
				}

				/* the current p has no shift on "error", pop stack */
				if yyDebug >= 2 {
					__yyfmt__.Printf("error recovery pops state %d\n", yyS[yyp].yys)
				}
				yyp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if yyDebug >= 2 {
				__yyfmt__.Printf("error recovery discards %s\n", yyTokname(yytoken))
			}
			if yytoken == yyEofCode {
				goto ret1
			}
			yyrcvr.char = -1
			yytoken = -1
			goto yynewstate /* try again in the same state */
		}
	}

	/* reduction by production yyn */
	if yyDebug >= 2 {
		__yyfmt__.Printf("reduce %v in:\n\t%v\n", yyn, yyStatname(yystate))
	}

	yynt := yyn
	yypt := yyp
	_ = yypt // guard against "declared and not used"

	yyp -= yyR2[yyn]
	// yyp is now the index of $0. Perform the default action. Iff the
	// reduced production is ε, $1 is possibly out of range.
	if yyp+1 >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyVAL = yyS[yyp+1]

	/* consult goto table to find next state */
	yyn = yyR1[yyn]
	yyg := yyPgo[yyn]
	yyj := yyg + yyS[yyp].yys + 1

	if yyj >= yyLast {
		yystate = yyAct[yyg]
	} else {
		yystate = yyAct[yyj]
		if yyChk[yystate] != -yyn {
			yystate = yyAct[yyg]
		}
	}
	// dummy call; replaced with literal code
	switch yynt {

	case 1:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:166
		{
			yylex.(*input).file = &File{Stmt: yyDollar[1].exprs}
			return 0
		}
	case 2:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:173
		{
			yyVAL.block = CodeBlock{
				Start:      yyDollar[2].pos,
				Statements: yyDollar[3].exprs,
				End:        End{Pos: yyDollar[4].pos},
			}
		}
	case 3:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:181
		{
			// simple_stmt is never empty
			start, _ := yyDollar[1].exprs[0].Span()
			_, end := yyDollar[1].exprs[len(yyDollar[1].exprs)-1].Span()
			yyVAL.block = CodeBlock{
				Start:      start,
				Statements: yyDollar[1].exprs,
				End:        End{Pos: end},
			}
		}
	case 4:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line build/parse.y:193
		{
			yyVAL.exprs = nil
			yyVAL.lastRule = nil
		}
	case 5:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:198
		{
			// If this statement follows a comment block,
			// attach the comments to the statement.
			if cb, ok := yyDollar[1].lastRule.(*CommentBlock); ok {
				yyVAL.exprs = append(yyDollar[1].exprs[:len(yyDollar[1].exprs)-1], yyDollar[2].exprs...)
				yyDollar[2].exprs[0].Comment().Before = cb.After
				yyVAL.lastRule = yyDollar[2].exprs[len(yyDollar[2].exprs)-1]
				break
			}

			// Otherwise add to list.
			yyVAL.exprs = append(yyDollar[1].exprs, yyDollar[2].exprs...)
			yyVAL.lastRule = yyDollar[2].exprs[len(yyDollar[2].exprs)-1]

			// Consider this input:
			//
			//	foo()
			//	# bar
			//	baz()
			//
			// If we've just parsed baz(), the # bar is attached to
			// foo() as an After comment. Make it a Before comment
			// for baz() instead.
			if x := yyDollar[1].lastRule; x != nil {
				com := x.Comment()
				// stmt is never empty
				yyDollar[2].exprs[0].Comment().Before = com.After
				com.After = nil
			}
		}
	case 6:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:229
		{
			// Blank line; sever last rule from future comments.
			yyVAL.exprs = yyDollar[1].exprs
			yyVAL.lastRule = nil
		}
	case 7:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:235
		{
			yyVAL.exprs = yyDollar[1].exprs
			yyVAL.lastRule = yyDollar[1].lastRule
			if yyVAL.lastRule == nil {
				cb := &CommentBlock{Start: yyDollar[2].pos}
				yyVAL.exprs = append(yyVAL.exprs, cb)
				yyVAL.lastRule = cb
			}
			com := yyVAL.lastRule.Comment()
			com.After = append(com.After, Comment{Start: yyDollar[2].pos, Token: yyDollar[2].tok})
		}
	case 8:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:249
		{
			yyVAL.exprs = yyDollar[1].exprs
		}
	case 9:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:253
		{
			yyVAL.exprs = []Expr{yyDollar[1].expr}
		}
	case 10:
		yyDollar = yyS[yypt-7 : yypt+1]
		//line build/parse.y:259
		{
			yyVAL.expr = &FuncDef{
				Start:          yyDollar[1].pos,
				Name:           yyDollar[2].tok,
				ListStart:      yyDollar[3].pos,
				Args:           yyDollar[4].exprs,
				Body:           yyDollar[7].block,
				End:            yyDollar[7].block.End,
				ForceCompact:   forceCompact(yyDollar[3].pos, yyDollar[4].exprs, yyDollar[5].pos),
				ForceMultiLine: forceMultiLine(yyDollar[3].pos, yyDollar[4].exprs, yyDollar[5].pos),
			}
		}
	case 11:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line build/parse.y:272
		{
			yyVAL.expr = &ForLoop{
				Start:    yyDollar[1].pos,
				LoopVars: yyDollar[2].exprs,
				Iterable: yyDollar[4].expr,
				Body:     yyDollar[6].block,
				End:      yyDollar[6].block.End,
			}
		}
	case 12:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:282
		{
			yyVAL.expr = yyDollar[1].expr
		}
	case 13:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:288
		{
			yyVAL.expr = &IfElse{
				Start: yyDollar[1].pos,
				Conditions: []Condition{
					Condition{
						If:   yyDollar[2].expr,
						Then: yyDollar[4].block,
					},
				},
				End: yyDollar[4].block.End,
			}
		}
	case 14:
		yyDollar = yyS[yypt-5 : yypt+1]
		//line build/parse.y:301
		{
			block := yyDollar[1].expr.(*IfElse)
			block.Conditions = append(block.Conditions, Condition{
				If:   yyDollar[3].expr,
				Then: yyDollar[5].block,
			})
			block.End = yyDollar[5].block.End
			yyVAL.expr = block
		}
	case 15:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:311
		{
			block := yyDollar[1].expr.(*IfElse)
			block.Conditions = append(block.Conditions, Condition{
				Then: yyDollar[4].block,
			})
			block.End = yyDollar[4].block.End
			yyVAL.expr = block
		}
	case 18:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:326
		{
			yyVAL.exprs = append([]Expr{yyDollar[1].expr}, yyDollar[2].exprs...)
			yyVAL.lastRule = yyVAL.exprs[len(yyVAL.exprs)-1]
		}
	case 19:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line build/parse.y:332
		{
			yyVAL.exprs = []Expr{}
		}
	case 20:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:336
		{
			yyVAL.exprs = append(yyDollar[1].exprs, yyDollar[3].expr)
		}
	case 22:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:343
		{
			_, end := yyDollar[2].expr.Span()
			yyVAL.expr = &ReturnExpr{
				X:   yyDollar[2].expr,
				End: end,
			}
		}
	case 23:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:351
		{
			yyVAL.expr = &ReturnExpr{End: yyDollar[1].pos}
		}
	case 24:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:355
		{
			yyVAL.expr = &PythonBlock{Start: yyDollar[1].pos, Token: yyDollar[1].tok}
		}
	case 28:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:365
		{
			yyVAL.expr = &DotExpr{
				X:       yyDollar[1].expr,
				Dot:     yyDollar[2].pos,
				NamePos: yyDollar[3].pos,
				Name:    yyDollar[3].tok,
			}
		}
	case 29:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:374
		{
			yyVAL.expr = &CallExpr{
				X:              yyDollar[1].expr,
				ListStart:      yyDollar[2].pos,
				List:           yyDollar[3].exprs,
				End:            End{Pos: yyDollar[4].pos},
				ForceCompact:   forceCompact(yyDollar[2].pos, yyDollar[3].exprs, yyDollar[4].pos),
				ForceMultiLine: forceMultiLine(yyDollar[2].pos, yyDollar[3].exprs, yyDollar[4].pos),
			}
		}
	case 30:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:385
		{
			yyVAL.expr = &IndexExpr{
				X:          yyDollar[1].expr,
				IndexStart: yyDollar[2].pos,
				Y:          yyDollar[3].expr,
				End:        yyDollar[4].pos,
			}
		}
	case 31:
		yyDollar = yyS[yypt-6 : yypt+1]
		//line build/parse.y:394
		{
			yyVAL.expr = &SliceExpr{
				X:          yyDollar[1].expr,
				SliceStart: yyDollar[2].pos,
				From:       yyDollar[3].expr,
				FirstColon: yyDollar[4].pos,
				To:         yyDollar[5].expr,
				End:        yyDollar[6].pos,
			}
		}
	case 32:
		yyDollar = yyS[yypt-8 : yypt+1]
		//line build/parse.y:405
		{
			yyVAL.expr = &SliceExpr{
				X:           yyDollar[1].expr,
				SliceStart:  yyDollar[2].pos,
				From:        yyDollar[3].expr,
				FirstColon:  yyDollar[4].pos,
				To:          yyDollar[5].expr,
				SecondColon: yyDollar[6].pos,
				Step:        yyDollar[7].expr,
				End:         yyDollar[8].pos,
			}
		}
	case 33:
		yyDollar = yyS[yypt-5 : yypt+1]
		//line build/parse.y:418
		{
			yyVAL.expr = &CallExpr{
				X:         yyDollar[1].expr,
				ListStart: yyDollar[2].pos,
				List: []Expr{
					&ListForExpr{
						Brack: "",
						Start: yyDollar[2].pos,
						X:     yyDollar[3].expr,
						For:   yyDollar[4].forsifs,
						End:   End{Pos: yyDollar[5].pos},
					},
				},
				End: End{Pos: yyDollar[5].pos},
			}
		}
	case 34:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:435
		{
			if len(yyDollar[1].strings) == 1 {
				yyVAL.expr = yyDollar[1].strings[0]
				break
			}
			yyVAL.expr = yyDollar[1].strings[0]
			for _, x := range yyDollar[1].strings[1:] {
				_, end := yyVAL.expr.Span()
				yyVAL.expr = binary(yyVAL.expr, end, "+", x)
			}
		}
	case 35:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:447
		{
			yyVAL.expr = &ListExpr{
				Start:          yyDollar[1].pos,
				List:           yyDollar[2].exprs,
				Comma:          yyDollar[2].comma,
				End:            End{Pos: yyDollar[3].pos},
				ForceMultiLine: forceMultiLine(yyDollar[1].pos, yyDollar[2].exprs, yyDollar[3].pos),
			}
		}
	case 36:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:457
		{
			exprStart, _ := yyDollar[2].expr.Span()
			yyVAL.expr = &ListForExpr{
				Brack:          "[]",
				Start:          yyDollar[1].pos,
				X:              yyDollar[2].expr,
				For:            yyDollar[3].forsifs,
				End:            End{Pos: yyDollar[4].pos},
				ForceMultiLine: yyDollar[1].pos.Line != exprStart.Line,
			}
		}
	case 37:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:469
		{
			exprStart, _ := yyDollar[2].expr.Span()
			yyVAL.expr = &ListForExpr{
				Brack:          "()",
				Start:          yyDollar[1].pos,
				X:              yyDollar[2].expr,
				For:            yyDollar[3].forsifs,
				End:            End{Pos: yyDollar[4].pos},
				ForceMultiLine: yyDollar[1].pos.Line != exprStart.Line,
			}
		}
	case 38:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:481
		{
			exprStart, _ := yyDollar[2].expr.Span()
			yyVAL.expr = &ListForExpr{
				Brack:          "{}",
				Start:          yyDollar[1].pos,
				X:              yyDollar[2].expr,
				For:            yyDollar[3].forsifs,
				End:            End{Pos: yyDollar[4].pos},
				ForceMultiLine: yyDollar[1].pos.Line != exprStart.Line,
			}
		}
	case 39:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:493
		{
			yyVAL.expr = &DictExpr{
				Start:          yyDollar[1].pos,
				List:           yyDollar[2].exprs,
				Comma:          yyDollar[2].comma,
				End:            End{Pos: yyDollar[3].pos},
				ForceMultiLine: forceMultiLine(yyDollar[1].pos, yyDollar[2].exprs, yyDollar[3].pos),
			}
		}
	case 40:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:503
		{
			yyVAL.expr = &SetExpr{
				Start:          yyDollar[1].pos,
				List:           yyDollar[2].exprs,
				Comma:          yyDollar[2].comma,
				End:            End{Pos: yyDollar[3].pos},
				ForceMultiLine: forceMultiLine(yyDollar[1].pos, yyDollar[2].exprs, yyDollar[3].pos),
			}
		}
	case 41:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:513
		{
			if len(yyDollar[2].exprs) == 1 && yyDollar[2].comma.Line == 0 {
				// Just a parenthesized expression, not a tuple.
				yyVAL.expr = &ParenExpr{
					Start:          yyDollar[1].pos,
					X:              yyDollar[2].exprs[0],
					End:            End{Pos: yyDollar[3].pos},
					ForceMultiLine: forceMultiLine(yyDollar[1].pos, yyDollar[2].exprs, yyDollar[3].pos),
				}
			} else {
				yyVAL.expr = &TupleExpr{
					Start:          yyDollar[1].pos,
					List:           yyDollar[2].exprs,
					Comma:          yyDollar[2].comma,
					End:            End{Pos: yyDollar[3].pos},
					ForceCompact:   forceCompact(yyDollar[1].pos, yyDollar[2].exprs, yyDollar[3].pos),
					ForceMultiLine: forceMultiLine(yyDollar[1].pos, yyDollar[2].exprs, yyDollar[3].pos),
				}
			}
		}
	case 42:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:533
		{
			yyVAL.expr = unary(yyDollar[1].pos, yyDollar[1].tok, yyDollar[2].expr)
		}
	case 44:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:538
		{
			yyVAL.expr = &LambdaExpr{
				Lambda: yyDollar[1].pos,
				Var:    yyDollar[2].exprs,
				Colon:  yyDollar[3].pos,
				Expr:   yyDollar[4].expr,
			}
		}
	case 45:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:546
		{
			yyVAL.expr = unary(yyDollar[1].pos, yyDollar[1].tok, yyDollar[2].expr)
		}
	case 46:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:547
		{
			yyVAL.expr = unary(yyDollar[1].pos, yyDollar[1].tok, yyDollar[2].expr)
		}
	case 47:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:548
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 48:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:549
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 49:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:550
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 50:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:551
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 51:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:552
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 52:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:553
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 53:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:554
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 54:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:555
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 55:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:556
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 56:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:557
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 57:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:558
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 58:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:559
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 59:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:560
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 60:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:561
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 61:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:562
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, "not in", yyDollar[4].expr)
		}
	case 62:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:563
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 63:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:564
		{
			yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
		}
	case 64:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:566
		{
			if b, ok := yyDollar[3].expr.(*UnaryExpr); ok && b.Op == "not" {
				yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, "is not", b.X)
			} else {
				yyVAL.expr = binary(yyDollar[1].expr, yyDollar[2].pos, yyDollar[2].tok, yyDollar[3].expr)
			}
		}
	case 65:
		yyDollar = yyS[yypt-5 : yypt+1]
		//line build/parse.y:574
		{
			yyVAL.expr = &ConditionalExpr{
				Then:      yyDollar[1].expr,
				IfStart:   yyDollar[2].pos,
				Test:      yyDollar[3].expr,
				ElseStart: yyDollar[4].pos,
				Else:      yyDollar[5].expr,
			}
		}
	case 66:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line build/parse.y:585
		{
			yyVAL.expr = nil
		}
	case 68:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line build/parse.y:595
		{
			yyVAL.pos = Position{}
		}
	case 70:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:601
		{
			yyVAL.expr = &KeyValueExpr{
				Key:   yyDollar[1].expr,
				Colon: yyDollar[2].pos,
				Value: yyDollar[3].expr,
			}
		}
	case 71:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:611
		{
			yyVAL.exprs = []Expr{yyDollar[1].expr}
		}
	case 72:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:615
		{
			yyVAL.exprs = append(yyDollar[1].exprs, yyDollar[3].expr)
		}
	case 73:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:621
		{
			yyVAL.exprs = yyDollar[1].exprs
		}
	case 74:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:625
		{
			yyVAL.exprs = yyDollar[1].exprs
		}
	case 75:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:631
		{
			yyVAL.exprs = []Expr{yyDollar[1].expr}
		}
	case 76:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:635
		{
			yyVAL.exprs = append(yyDollar[1].exprs, yyDollar[3].expr)
		}
	case 77:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line build/parse.y:640
		{
			yyVAL.exprs, yyVAL.comma = nil, Position{}
		}
	case 78:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:644
		{
			yyVAL.exprs, yyVAL.comma = yyDollar[1].exprs, yyDollar[2].pos
		}
	case 79:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:650
		{
			yyVAL.exprs = []Expr{yyDollar[1].expr}
		}
	case 80:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:654
		{
			yyVAL.exprs = append(yyDollar[1].exprs, yyDollar[3].expr)
		}
	case 81:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:660
		{
			yyVAL.string = &StringExpr{
				Start:       yyDollar[1].pos,
				Value:       yyDollar[1].str,
				TripleQuote: yyDollar[1].triple,
				End:         yyDollar[1].pos.add(yyDollar[1].tok),
				Token:       yyDollar[1].tok,
			}
		}
	case 82:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:672
		{
			yyVAL.strings = []*StringExpr{yyDollar[1].string}
		}
	case 83:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:676
		{
			yyVAL.strings = append(yyDollar[1].strings, yyDollar[2].string)
		}
	case 84:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:682
		{
			yyVAL.expr = &LiteralExpr{Start: yyDollar[1].pos, Token: yyDollar[1].tok}
		}
	case 85:
		yyDollar = yyS[yypt-4 : yypt+1]
		//line build/parse.y:688
		{
			yyVAL.forc = &ForClause{
				For:  yyDollar[1].pos,
				Var:  yyDollar[2].exprs,
				In:   yyDollar[3].pos,
				Expr: yyDollar[4].expr,
			}
		}
	case 86:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:698
		{
			yyVAL.forifs = &ForClauseWithIfClausesOpt{
				For: yyDollar[1].forc,
				Ifs: yyDollar[2].ifs,
			}
		}
	case 87:
		yyDollar = yyS[yypt-1 : yypt+1]
		//line build/parse.y:707
		{
			yyVAL.forsifs = []*ForClauseWithIfClausesOpt{yyDollar[1].forifs}
		}
	case 88:
		yyDollar = yyS[yypt-2 : yypt+1]
		//line build/parse.y:710
		{
			yyVAL.forsifs = append(yyDollar[1].forsifs, yyDollar[2].forifs)
		}
	case 89:
		yyDollar = yyS[yypt-0 : yypt+1]
		//line build/parse.y:715
		{
			yyVAL.ifs = nil
		}
	case 90:
		yyDollar = yyS[yypt-3 : yypt+1]
		//line build/parse.y:719
		{
			yyVAL.ifs = append(yyDollar[1].ifs, &IfClause{
				If:   yyDollar[2].pos,
				Cond: yyDollar[3].expr,
			})
		}
	}
	goto yystack /* stack new state and value */
}
