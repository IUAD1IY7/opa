// Copyright 2024 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package ast

import (
	"strconv"
	"unique"
)

type internable interface {
	bool | string | int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64
}

// NOTE! Great care must be taken **not** to modify the terms returned
// from these functions, as they are shared across all callers.
// This package is currently considered experimental, and may change
// at any time without notice.

var (
	InternedNullValue Value = Null{}
	InternedNullTerm        = &Term{Value: InternedNullValue}

	InternedBooleanTrueValue  Value = Boolean(true)
	InternedBooleanFalseValue Value = Boolean(false)
	InternedBooleanTrueTerm         = &Term{Value: InternedBooleanTrueValue}
	InternedBooleanFalseTerm        = &Term{Value: InternedBooleanFalseValue}

	InternedEmptyString = &Term{Value: String("")}
	InternedEmptyObject = ObjectTerm()
	InternedEmptyArray  = NewTerm(InternedEmptyArrayValue)
	InternedEmptySet    = SetTerm()

	InternedEmptyArrayValue = NewArray()

	// since this is by far the most common negative number
	minusOneValue Value = Number("-1")
	minusOneTerm        = &Term{Value: minusOneValue}

	internedStringTerms = map[unique.Handle[string]]*Term{
		unique.Make(""): InternedEmptyString,
	}

	internedVarValues = map[string]Value{
		"input": Var("input"),
		"data":  Var("data"),
		"key":   Var("key"),
		"value": Var("value"),

		"i": Var("i"), "j": Var("j"), "k": Var("k"), "v": Var("v"), "x": Var("x"), "y": Var("y"), "z": Var("z"),
	}
)

// InternStringTerm interns the given strings as terms. Note that Interning is
// considered experimental and should not be relied upon by external code.
// WARNING: This must **only** be called at initialization time, as the
// interned terms are shared globally, and the underlying map is not thread-safe.
func InternStringTerm(str ...string) {
	for _, s := range str {
		h := unique.Make(s)
		if _, ok := internedStringTerms[h]; ok {
			continue
		}

		internedStringTerms[h] = &Term{Value: String(s)}
	}
}

// InternVarValue interns the given variable names as Var Values. Note that Interning is
// considered experimental and should not be relied upon by external code.
// WARNING: This must **only** be called at initialization time, as the
// interned terms are shared globally, and the underlying map is not thread-safe.
func InternVarValue(names ...string) {
	for _, name := range names {
		if _, ok := internedVarValues[name]; ok {
			continue
		}

		internedVarValues[name] = Var(name)
	}
}

// HasInternedValue returns true if the given value is interned, otherwise false.
func HasInternedValue[T internable](v T) bool {
	switch value := any(v).(type) {
	case bool:
		return true
	case int:
		return HasInternedIntNumberTerm(value)
	case int8:
		return HasInternedIntNumberTerm(int(value))
	case int16:
		return HasInternedIntNumberTerm(int(value))
	case int32:
		return HasInternedIntNumberTerm(int(value))
	case int64:
		return HasInternedIntNumberTerm(int(value))
	case uint:
		return HasInternedIntNumberTerm(int(value))
	case uint8:
		return HasInternedIntNumberTerm(int(value))
	case uint16:
		return HasInternedIntNumberTerm(int(value))
	case uint32:
		return HasInternedIntNumberTerm(int(value))
	case uint64:
		return HasInternedIntNumberTerm(int(value))
	case string:
		h := unique.Make(value)
		_, ok := internedStringTerms[h]
		return ok
	}
	return false
}

// InternedValue returns an interned Value for scalar v, if the value is
// interned. If the value is not interned, a new Value is returned.
func InternedValue[T internable](v T) Value {
	return InternedValueOr(v, internedTermValue)
}

// InternedVarValue returns an interned Var Value for the given name. If the
// name is not interned, a new Var Value is returned.
func InternedVarValue(name string) Value {
	if v, ok := internedVarValues[name]; ok {
		return v
	}

	return Var(name)
}

// InternedValueOr returns an interned Value for scalar v. Calls supplier
// to produce a Value if the value is not interned.
func InternedValueOr[T internable](v T, supplier func(T) Value) Value {
	switch value := any(v).(type) {
	case bool:
		return internedBooleanValue(value)
	case int:
		return internedIntNumberValue(value)
	case int8:
		return internedIntNumberValue(int(value))
	case int16:
		return internedIntNumberValue(int(value))
	case int32:
		return internedIntNumberValue(int(value))
	case int64:
		return internedIntNumberValue(int(value))
	case uint:
		return internedIntNumberValue(int(value))
	case uint8:
		return internedIntNumberValue(int(value))
	case uint16:
		return internedIntNumberValue(int(value))
	case uint32:
		return internedIntNumberValue(int(value))
	case uint64:
		return internedIntNumberValue(int(value))
	}
	return supplier(v)
}

// Interned returns a possibly interned term for the given scalar value.
// If the value is not interned, a new term is created for that value.
func InternedTerm[T internable](v T) *Term {
	switch value := any(v).(type) {
	case bool:
		return internedBooleanTerm(value)
	case string:
		return internedStringTerm(value)
	case int:
		return internedIntNumberTerm(value)
	case int8:
		return internedIntNumberTerm(int(value))
	case int16:
		return internedIntNumberTerm(int(value))
	case int32:
		return internedIntNumberTerm(int(value))
	case int64:
		return internedIntNumberTerm(int(value))
	case uint:
		return internedIntNumberTerm(int(value))
	case uint8:
		return internedIntNumberTerm(int(value))
	case uint16:
		return internedIntNumberTerm(int(value))
	case uint32:
		return internedIntNumberTerm(int(value))
	case uint64:
		return internedIntNumberTerm(int(value))
	default:
		panic("unreachable")
	}
}

// InternedIntFromString returns a term with the given integer value if the string
// maps to an interned term. If the string does not map to an interned term, nil is
// returned.
func InternedIntNumberTermFromString(s string) *Term {
	h := unique.Make(s)
	if term, ok := stringToIntNumberTermMap[h]; ok {
		return term
	}

	return nil
}

// HasInternedIntNumberTerm returns true if the given integer value maps to an interned
// term, otherwise false.
func HasInternedIntNumberTerm(i int) bool {
	return i >= -1 && i < len(intNumberTerms)
}

// Returns an interned string term representing the integer value i, if
// interned. If not, creates a new StringTerm for the integer value.
func InternedIntegerString(i int) *Term {
	// Cheapest option - we don't need to call strconv.Itoa
	if HasInternedIntNumberTerm(i) {
		str := IntNumberTerm(i).String()
		h := unique.Make(str)
		if interned, ok := internedStringTerms[h]; ok {
			return interned
		}
	}

	// Next cheapest option — the string could still be interned if the store
	// has been extended with more terms than we cucrrently intern.
	s := strconv.Itoa(i)
	h := unique.Make(s)
	if interned, ok := internedStringTerms[h]; ok {
		return interned
	}

	// Nope, create a new term
	return StringTerm(s)
}

func internedBooleanValue(b bool) Value {
	if b {
		return InternedBooleanTrueValue
	}

	return InternedBooleanFalseValue
}

// InternedBooleanTerm returns an interned term with the given boolean value.
func internedBooleanTerm(b bool) *Term {
	if b {
		return InternedBooleanTrueTerm
	}

	return InternedBooleanFalseTerm
}

func internedIntNumberValue(i int) Value {
	if i >= 0 && i < len(intNumberTerms) {
		return intNumberValues[i]
	}

	if i == -1 {
		return minusOneValue
	}

	return Number(strconv.Itoa(i))
}

// InternedIntNumberTerm returns a term with the given integer value. The term is
// cached between -1 to 512, and for values outside of that range, this function
// is equivalent to IntNumberTerm.
func internedIntNumberTerm(i int) *Term {
	if i >= 0 && i < len(intNumberTerms) {
		return intNumberTerms[i]
	}

	if i == -1 {
		return minusOneTerm
	}

	return &Term{Value: Number(strconv.Itoa(i))}
}

// InternedStringTerm returns an interned term with the given string value. If the
// provided string is not interned, a new term is created for that value. It does *not*
// modify the global interned terms map.
func internedStringTerm(s string) *Term {
	h := unique.Make(s)
	if term, ok := internedStringTerms[h]; ok {
		return term
	}

	return &Term{Value: String(s)}
}

func internedTermValue[T internable](v T) Value {
	return InternedTerm(v).Value
}

func init() {
	InternStringTerm(
		// Numbers
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
		"11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
		"21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31", "32", "33", "34", "35", "36", "37", "38",
		"39", "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "50", "51", "52", "53", "54", "55", "56",
		"57", "58", "59", "60", "61", "62", "63", "64", "65", "66", "67", "68", "69", "70", "71", "72", "73", "74",
		"75", "76", "77", "78", "79", "80", "81", "82", "83", "84", "85", "86", "87", "88", "89", "90", "91", "92",
		"93", "94", "95", "96", "97", "98", "99", "100",
		// Types
		"null", "boolean", "number", "string", "array", "object", "set", "var", "ref", "true", "false",
		// Runtime
		"config", "env", "version", "commit", "authorization_enabled", "skip_known_schema_check",
		// Annotations
		"annotations", "scope", "title", "entrypoint", "description", "organizations", "authors", "related_resources",
		"schemas", "custom", "name", "email", "schema", "definition", "document", "package", "rule", "subpackages",
		// Debug
		"text", "value", "bindings", "expressions",
		// Various
		"data", "input", "result", "keywords", "path", "v1", "error", "partial",
		// HTTP
		"code", "message", "status_code", "method", "url", "uri",
		// JWT
		"enc", "cty", "iss", "exp", "nbf", "aud", "secret", "cert",
		// Decisions
		"revision", "labels", "decision_id", "bundles", "query", "mapped_result", "nd_builtin_cache",
		"erased", "masked", "requested_by", "timestamp", "metrics", "req_id",

		// Whitespace
		" ", "\n", "\t",
	)
}

var stringToIntNumberTermMap = make(map[unique.Handle[string]]*Term, 514)

func init() {
	// Initialize string-to-intNumberTerm map with unique.Handle keys
	stringToIntNumberTermMap[unique.Make("-1")] = minusOneTerm
	for i := 0; i <= 512; i++ {
		s := strconv.Itoa(i)
		stringToIntNumberTermMap[unique.Make(s)] = intNumberTerms[i]
	}
}


var intNumberValues = [...]Value{
	Number("0"),
	Number("1"),
	Number("2"),
	Number("3"),
	Number("4"),
	Number("5"),
	Number("6"),
	Number("7"),
	Number("8"),
	Number("9"),
	Number("10"),
	Number("11"),
	Number("12"),
	Number("13"),
	Number("14"),
	Number("15"),
	Number("16"),
	Number("17"),
	Number("18"),
	Number("19"),
	Number("20"),
	Number("21"),
	Number("22"),
	Number("23"),
	Number("24"),
	Number("25"),
	Number("26"),
	Number("27"),
	Number("28"),
	Number("29"),
	Number("30"),
	Number("31"),
	Number("32"),
	Number("33"),
	Number("34"),
	Number("35"),
	Number("36"),
	Number("37"),
	Number("38"),
	Number("39"),
	Number("40"),
	Number("41"),
	Number("42"),
	Number("43"),
	Number("44"),
	Number("45"),
	Number("46"),
	Number("47"),
	Number("48"),
	Number("49"),
	Number("50"),
	Number("51"),
	Number("52"),
	Number("53"),
	Number("54"),
	Number("55"),
	Number("56"),
	Number("57"),
	Number("58"),
	Number("59"),
	Number("60"),
	Number("61"),
	Number("62"),
	Number("63"),
	Number("64"),
	Number("65"),
	Number("66"),
	Number("67"),
	Number("68"),
	Number("69"),
	Number("70"),
	Number("71"),
	Number("72"),
	Number("73"),
	Number("74"),
	Number("75"),
	Number("76"),
	Number("77"),
	Number("78"),
	Number("79"),
	Number("80"),
	Number("81"),
	Number("82"),
	Number("83"),
	Number("84"),
	Number("85"),
	Number("86"),
	Number("87"),
	Number("88"),
	Number("89"),
	Number("90"),
	Number("91"),
	Number("92"),
	Number("93"),
	Number("94"),
	Number("95"),
	Number("96"),
	Number("97"),
	Number("98"),
	Number("99"),
	Number("100"),
	Number("101"),
	Number("102"),
	Number("103"),
	Number("104"),
	Number("105"),
	Number("106"),
	Number("107"),
	Number("108"),
	Number("109"),
	Number("110"),
	Number("111"),
	Number("112"),
	Number("113"),
	Number("114"),
	Number("115"),
	Number("116"),
	Number("117"),
	Number("118"),
	Number("119"),
	Number("120"),
	Number("121"),
	Number("122"),
	Number("123"),
	Number("124"),
	Number("125"),
	Number("126"),
	Number("127"),
	Number("128"),
	Number("129"),
	Number("130"),
	Number("131"),
	Number("132"),
	Number("133"),
	Number("134"),
	Number("135"),
	Number("136"),
	Number("137"),
	Number("138"),
	Number("139"),
	Number("140"),
	Number("141"),
	Number("142"),
	Number("143"),
	Number("144"),
	Number("145"),
	Number("146"),
	Number("147"),
	Number("148"),
	Number("149"),
	Number("150"),
	Number("151"),
	Number("152"),
	Number("153"),
	Number("154"),
	Number("155"),
	Number("156"),
	Number("157"),
	Number("158"),
	Number("159"),
	Number("160"),
	Number("161"),
	Number("162"),
	Number("163"),
	Number("164"),
	Number("165"),
	Number("166"),
	Number("167"),
	Number("168"),
	Number("169"),
	Number("170"),
	Number("171"),
	Number("172"),
	Number("173"),
	Number("174"),
	Number("175"),
	Number("176"),
	Number("177"),
	Number("178"),
	Number("179"),
	Number("180"),
	Number("181"),
	Number("182"),
	Number("183"),
	Number("184"),
	Number("185"),
	Number("186"),
	Number("187"),
	Number("188"),
	Number("189"),
	Number("190"),
	Number("191"),
	Number("192"),
	Number("193"),
	Number("194"),
	Number("195"),
	Number("196"),
	Number("197"),
	Number("198"),
	Number("199"),
	Number("200"),
	Number("201"),
	Number("202"),
	Number("203"),
	Number("204"),
	Number("205"),
	Number("206"),
	Number("207"),
	Number("208"),
	Number("209"),
	Number("210"),
	Number("211"),
	Number("212"),
	Number("213"),
	Number("214"),
	Number("215"),
	Number("216"),
	Number("217"),
	Number("218"),
	Number("219"),
	Number("220"),
	Number("221"),
	Number("222"),
	Number("223"),
	Number("224"),
	Number("225"),
	Number("226"),
	Number("227"),
	Number("228"),
	Number("229"),
	Number("230"),
	Number("231"),
	Number("232"),
	Number("233"),
	Number("234"),
	Number("235"),
	Number("236"),
	Number("237"),
	Number("238"),
	Number("239"),
	Number("240"),
	Number("241"),
	Number("242"),
	Number("243"),
	Number("244"),
	Number("245"),
	Number("246"),
	Number("247"),
	Number("248"),
	Number("249"),
	Number("250"),
	Number("251"),
	Number("252"),
	Number("253"),
	Number("254"),
	Number("255"),
	Number("256"),
	Number("257"),
	Number("258"),
	Number("259"),
	Number("260"),
	Number("261"),
	Number("262"),
	Number("263"),
	Number("264"),
	Number("265"),
	Number("266"),
	Number("267"),
	Number("268"),
	Number("269"),
	Number("270"),
	Number("271"),
	Number("272"),
	Number("273"),
	Number("274"),
	Number("275"),
	Number("276"),
	Number("277"),
	Number("278"),
	Number("279"),
	Number("280"),
	Number("281"),
	Number("282"),
	Number("283"),
	Number("284"),
	Number("285"),
	Number("286"),
	Number("287"),
	Number("288"),
	Number("289"),
	Number("290"),
	Number("291"),
	Number("292"),
	Number("293"),
	Number("294"),
	Number("295"),
	Number("296"),
	Number("297"),
	Number("298"),
	Number("299"),
	Number("300"),
	Number("301"),
	Number("302"),
	Number("303"),
	Number("304"),
	Number("305"),
	Number("306"),
	Number("307"),
	Number("308"),
	Number("309"),
	Number("310"),
	Number("311"),
	Number("312"),
	Number("313"),
	Number("314"),
	Number("315"),
	Number("316"),
	Number("317"),
	Number("318"),
	Number("319"),
	Number("320"),
	Number("321"),
	Number("322"),
	Number("323"),
	Number("324"),
	Number("325"),
	Number("326"),
	Number("327"),
	Number("328"),
	Number("329"),
	Number("330"),
	Number("331"),
	Number("332"),
	Number("333"),
	Number("334"),
	Number("335"),
	Number("336"),
	Number("337"),
	Number("338"),
	Number("339"),
	Number("340"),
	Number("341"),
	Number("342"),
	Number("343"),
	Number("344"),
	Number("345"),
	Number("346"),
	Number("347"),
	Number("348"),
	Number("349"),
	Number("350"),
	Number("351"),
	Number("352"),
	Number("353"),
	Number("354"),
	Number("355"),
	Number("356"),
	Number("357"),
	Number("358"),
	Number("359"),
	Number("360"),
	Number("361"),
	Number("362"),
	Number("363"),
	Number("364"),
	Number("365"),
	Number("366"),
	Number("367"),
	Number("368"),
	Number("369"),
	Number("370"),
	Number("371"),
	Number("372"),
	Number("373"),
	Number("374"),
	Number("375"),
	Number("376"),
	Number("377"),
	Number("378"),
	Number("379"),
	Number("380"),
	Number("381"),
	Number("382"),
	Number("383"),
	Number("384"),
	Number("385"),
	Number("386"),
	Number("387"),
	Number("388"),
	Number("389"),
	Number("390"),
	Number("391"),
	Number("392"),
	Number("393"),
	Number("394"),
	Number("395"),
	Number("396"),
	Number("397"),
	Number("398"),
	Number("399"),
	Number("400"),
	Number("401"),
	Number("402"),
	Number("403"),
	Number("404"),
	Number("405"),
	Number("406"),
	Number("407"),
	Number("408"),
	Number("409"),
	Number("410"),
	Number("411"),
	Number("412"),
	Number("413"),
	Number("414"),
	Number("415"),
	Number("416"),
	Number("417"),
	Number("418"),
	Number("419"),
	Number("420"),
	Number("421"),
	Number("422"),
	Number("423"),
	Number("424"),
	Number("425"),
	Number("426"),
	Number("427"),
	Number("428"),
	Number("429"),
	Number("430"),
	Number("431"),
	Number("432"),
	Number("433"),
	Number("434"),
	Number("435"),
	Number("436"),
	Number("437"),
	Number("438"),
	Number("439"),
	Number("440"),
	Number("441"),
	Number("442"),
	Number("443"),
	Number("444"),
	Number("445"),
	Number("446"),
	Number("447"),
	Number("448"),
	Number("449"),
	Number("450"),
	Number("451"),
	Number("452"),
	Number("453"),
	Number("454"),
	Number("455"),
	Number("456"),
	Number("457"),
	Number("458"),
	Number("459"),
	Number("460"),
	Number("461"),
	Number("462"),
	Number("463"),
	Number("464"),
	Number("465"),
	Number("466"),
	Number("467"),
	Number("468"),
	Number("469"),
	Number("470"),
	Number("471"),
	Number("472"),
	Number("473"),
	Number("474"),
	Number("475"),
	Number("476"),
	Number("477"),
	Number("478"),
	Number("479"),
	Number("480"),
	Number("481"),
	Number("482"),
	Number("483"),
	Number("484"),
	Number("485"),
	Number("486"),
	Number("487"),
	Number("488"),
	Number("489"),
	Number("490"),
	Number("491"),
	Number("492"),
	Number("493"),
	Number("494"),
	Number("495"),
	Number("496"),
	Number("497"),
	Number("498"),
	Number("499"),
	Number("500"),
	Number("501"),
	Number("502"),
	Number("503"),
	Number("504"),
	Number("505"),
	Number("506"),
	Number("507"),
	Number("508"),
	Number("509"),
	Number("510"),
	Number("511"),
	Number("512"),
}

var intNumberTerms = [...]*Term{
	{Value: intNumberValues[0]},
	{Value: intNumberValues[1]},
	{Value: intNumberValues[2]},
	{Value: intNumberValues[3]},
	{Value: intNumberValues[4]},
	{Value: intNumberValues[5]},
	{Value: intNumberValues[6]},
	{Value: intNumberValues[7]},
	{Value: intNumberValues[8]},
	{Value: intNumberValues[9]},
	{Value: intNumberValues[10]},
	{Value: intNumberValues[11]},
	{Value: intNumberValues[12]},
	{Value: intNumberValues[13]},
	{Value: intNumberValues[14]},
	{Value: intNumberValues[15]},
	{Value: intNumberValues[16]},
	{Value: intNumberValues[17]},
	{Value: intNumberValues[18]},
	{Value: intNumberValues[19]},
	{Value: intNumberValues[20]},
	{Value: intNumberValues[21]},
	{Value: intNumberValues[22]},
	{Value: intNumberValues[23]},
	{Value: intNumberValues[24]},
	{Value: intNumberValues[25]},
	{Value: intNumberValues[26]},
	{Value: intNumberValues[27]},
	{Value: intNumberValues[28]},
	{Value: intNumberValues[29]},
	{Value: intNumberValues[30]},
	{Value: intNumberValues[31]},
	{Value: intNumberValues[32]},
	{Value: intNumberValues[33]},
	{Value: intNumberValues[34]},
	{Value: intNumberValues[35]},
	{Value: intNumberValues[36]},
	{Value: intNumberValues[37]},
	{Value: intNumberValues[38]},
	{Value: intNumberValues[39]},
	{Value: intNumberValues[40]},
	{Value: intNumberValues[41]},
	{Value: intNumberValues[42]},
	{Value: intNumberValues[43]},
	{Value: intNumberValues[44]},
	{Value: intNumberValues[45]},
	{Value: intNumberValues[46]},
	{Value: intNumberValues[47]},
	{Value: intNumberValues[48]},
	{Value: intNumberValues[49]},
	{Value: intNumberValues[50]},
	{Value: intNumberValues[51]},
	{Value: intNumberValues[52]},
	{Value: intNumberValues[53]},
	{Value: intNumberValues[54]},
	{Value: intNumberValues[55]},
	{Value: intNumberValues[56]},
	{Value: intNumberValues[57]},
	{Value: intNumberValues[58]},
	{Value: intNumberValues[59]},
	{Value: intNumberValues[60]},
	{Value: intNumberValues[61]},
	{Value: intNumberValues[62]},
	{Value: intNumberValues[63]},
	{Value: intNumberValues[64]},
	{Value: intNumberValues[65]},
	{Value: intNumberValues[66]},
	{Value: intNumberValues[67]},
	{Value: intNumberValues[68]},
	{Value: intNumberValues[69]},
	{Value: intNumberValues[70]},
	{Value: intNumberValues[71]},
	{Value: intNumberValues[72]},
	{Value: intNumberValues[73]},
	{Value: intNumberValues[74]},
	{Value: intNumberValues[75]},
	{Value: intNumberValues[76]},
	{Value: intNumberValues[77]},
	{Value: intNumberValues[78]},
	{Value: intNumberValues[79]},
	{Value: intNumberValues[80]},
	{Value: intNumberValues[81]},
	{Value: intNumberValues[82]},
	{Value: intNumberValues[83]},
	{Value: intNumberValues[84]},
	{Value: intNumberValues[85]},
	{Value: intNumberValues[86]},
	{Value: intNumberValues[87]},
	{Value: intNumberValues[88]},
	{Value: intNumberValues[89]},
	{Value: intNumberValues[90]},
	{Value: intNumberValues[91]},
	{Value: intNumberValues[92]},
	{Value: intNumberValues[93]},
	{Value: intNumberValues[94]},
	{Value: intNumberValues[95]},
	{Value: intNumberValues[96]},
	{Value: intNumberValues[97]},
	{Value: intNumberValues[98]},
	{Value: intNumberValues[99]},
	{Value: intNumberValues[100]},
	{Value: intNumberValues[101]},
	{Value: intNumberValues[102]},
	{Value: intNumberValues[103]},
	{Value: intNumberValues[104]},
	{Value: intNumberValues[105]},
	{Value: intNumberValues[106]},
	{Value: intNumberValues[107]},
	{Value: intNumberValues[108]},
	{Value: intNumberValues[109]},
	{Value: intNumberValues[110]},
	{Value: intNumberValues[111]},
	{Value: intNumberValues[112]},
	{Value: intNumberValues[113]},
	{Value: intNumberValues[114]},
	{Value: intNumberValues[115]},
	{Value: intNumberValues[116]},
	{Value: intNumberValues[117]},
	{Value: intNumberValues[118]},
	{Value: intNumberValues[119]},
	{Value: intNumberValues[120]},
	{Value: intNumberValues[121]},
	{Value: intNumberValues[122]},
	{Value: intNumberValues[123]},
	{Value: intNumberValues[124]},
	{Value: intNumberValues[125]},
	{Value: intNumberValues[126]},
	{Value: intNumberValues[127]},
	{Value: intNumberValues[128]},
	{Value: intNumberValues[129]},
	{Value: intNumberValues[130]},
	{Value: intNumberValues[131]},
	{Value: intNumberValues[132]},
	{Value: intNumberValues[133]},
	{Value: intNumberValues[134]},
	{Value: intNumberValues[135]},
	{Value: intNumberValues[136]},
	{Value: intNumberValues[137]},
	{Value: intNumberValues[138]},
	{Value: intNumberValues[139]},
	{Value: intNumberValues[140]},
	{Value: intNumberValues[141]},
	{Value: intNumberValues[142]},
	{Value: intNumberValues[143]},
	{Value: intNumberValues[144]},
	{Value: intNumberValues[145]},
	{Value: intNumberValues[146]},
	{Value: intNumberValues[147]},
	{Value: intNumberValues[148]},
	{Value: intNumberValues[149]},
	{Value: intNumberValues[150]},
	{Value: intNumberValues[151]},
	{Value: intNumberValues[152]},
	{Value: intNumberValues[153]},
	{Value: intNumberValues[154]},
	{Value: intNumberValues[155]},
	{Value: intNumberValues[156]},
	{Value: intNumberValues[157]},
	{Value: intNumberValues[158]},
	{Value: intNumberValues[159]},
	{Value: intNumberValues[160]},
	{Value: intNumberValues[161]},
	{Value: intNumberValues[162]},
	{Value: intNumberValues[163]},
	{Value: intNumberValues[164]},
	{Value: intNumberValues[165]},
	{Value: intNumberValues[166]},
	{Value: intNumberValues[167]},
	{Value: intNumberValues[168]},
	{Value: intNumberValues[169]},
	{Value: intNumberValues[170]},
	{Value: intNumberValues[171]},
	{Value: intNumberValues[172]},
	{Value: intNumberValues[173]},
	{Value: intNumberValues[174]},
	{Value: intNumberValues[175]},
	{Value: intNumberValues[176]},
	{Value: intNumberValues[177]},
	{Value: intNumberValues[178]},
	{Value: intNumberValues[179]},
	{Value: intNumberValues[180]},
	{Value: intNumberValues[181]},
	{Value: intNumberValues[182]},
	{Value: intNumberValues[183]},
	{Value: intNumberValues[184]},
	{Value: intNumberValues[185]},
	{Value: intNumberValues[186]},
	{Value: intNumberValues[187]},
	{Value: intNumberValues[188]},
	{Value: intNumberValues[189]},
	{Value: intNumberValues[190]},
	{Value: intNumberValues[191]},
	{Value: intNumberValues[192]},
	{Value: intNumberValues[193]},
	{Value: intNumberValues[194]},
	{Value: intNumberValues[195]},
	{Value: intNumberValues[196]},
	{Value: intNumberValues[197]},
	{Value: intNumberValues[198]},
	{Value: intNumberValues[199]},
	{Value: intNumberValues[200]},
	{Value: intNumberValues[201]},
	{Value: intNumberValues[202]},
	{Value: intNumberValues[203]},
	{Value: intNumberValues[204]},
	{Value: intNumberValues[205]},
	{Value: intNumberValues[206]},
	{Value: intNumberValues[207]},
	{Value: intNumberValues[208]},
	{Value: intNumberValues[209]},
	{Value: intNumberValues[210]},
	{Value: intNumberValues[211]},
	{Value: intNumberValues[212]},
	{Value: intNumberValues[213]},
	{Value: intNumberValues[214]},
	{Value: intNumberValues[215]},
	{Value: intNumberValues[216]},
	{Value: intNumberValues[217]},
	{Value: intNumberValues[218]},
	{Value: intNumberValues[219]},
	{Value: intNumberValues[220]},
	{Value: intNumberValues[221]},
	{Value: intNumberValues[222]},
	{Value: intNumberValues[223]},
	{Value: intNumberValues[224]},
	{Value: intNumberValues[225]},
	{Value: intNumberValues[226]},
	{Value: intNumberValues[227]},
	{Value: intNumberValues[228]},
	{Value: intNumberValues[229]},
	{Value: intNumberValues[230]},
	{Value: intNumberValues[231]},
	{Value: intNumberValues[232]},
	{Value: intNumberValues[233]},
	{Value: intNumberValues[234]},
	{Value: intNumberValues[235]},
	{Value: intNumberValues[236]},
	{Value: intNumberValues[237]},
	{Value: intNumberValues[238]},
	{Value: intNumberValues[239]},
	{Value: intNumberValues[240]},
	{Value: intNumberValues[241]},
	{Value: intNumberValues[242]},
	{Value: intNumberValues[243]},
	{Value: intNumberValues[244]},
	{Value: intNumberValues[245]},
	{Value: intNumberValues[246]},
	{Value: intNumberValues[247]},
	{Value: intNumberValues[248]},
	{Value: intNumberValues[249]},
	{Value: intNumberValues[250]},
	{Value: intNumberValues[251]},
	{Value: intNumberValues[252]},
	{Value: intNumberValues[253]},
	{Value: intNumberValues[254]},
	{Value: intNumberValues[255]},
	{Value: intNumberValues[256]},
	{Value: intNumberValues[257]},
	{Value: intNumberValues[258]},
	{Value: intNumberValues[259]},
	{Value: intNumberValues[260]},
	{Value: intNumberValues[261]},
	{Value: intNumberValues[262]},
	{Value: intNumberValues[263]},
	{Value: intNumberValues[264]},
	{Value: intNumberValues[265]},
	{Value: intNumberValues[266]},
	{Value: intNumberValues[267]},
	{Value: intNumberValues[268]},
	{Value: intNumberValues[269]},
	{Value: intNumberValues[270]},
	{Value: intNumberValues[271]},
	{Value: intNumberValues[272]},
	{Value: intNumberValues[273]},
	{Value: intNumberValues[274]},
	{Value: intNumberValues[275]},
	{Value: intNumberValues[276]},
	{Value: intNumberValues[277]},
	{Value: intNumberValues[278]},
	{Value: intNumberValues[279]},
	{Value: intNumberValues[280]},
	{Value: intNumberValues[281]},
	{Value: intNumberValues[282]},
	{Value: intNumberValues[283]},
	{Value: intNumberValues[284]},
	{Value: intNumberValues[285]},
	{Value: intNumberValues[286]},
	{Value: intNumberValues[287]},
	{Value: intNumberValues[288]},
	{Value: intNumberValues[289]},
	{Value: intNumberValues[290]},
	{Value: intNumberValues[291]},
	{Value: intNumberValues[292]},
	{Value: intNumberValues[293]},
	{Value: intNumberValues[294]},
	{Value: intNumberValues[295]},
	{Value: intNumberValues[296]},
	{Value: intNumberValues[297]},
	{Value: intNumberValues[298]},
	{Value: intNumberValues[299]},
	{Value: intNumberValues[300]},
	{Value: intNumberValues[301]},
	{Value: intNumberValues[302]},
	{Value: intNumberValues[303]},
	{Value: intNumberValues[304]},
	{Value: intNumberValues[305]},
	{Value: intNumberValues[306]},
	{Value: intNumberValues[307]},
	{Value: intNumberValues[308]},
	{Value: intNumberValues[309]},
	{Value: intNumberValues[310]},
	{Value: intNumberValues[311]},
	{Value: intNumberValues[312]},
	{Value: intNumberValues[313]},
	{Value: intNumberValues[314]},
	{Value: intNumberValues[315]},
	{Value: intNumberValues[316]},
	{Value: intNumberValues[317]},
	{Value: intNumberValues[318]},
	{Value: intNumberValues[319]},
	{Value: intNumberValues[320]},
	{Value: intNumberValues[321]},
	{Value: intNumberValues[322]},
	{Value: intNumberValues[323]},
	{Value: intNumberValues[324]},
	{Value: intNumberValues[325]},
	{Value: intNumberValues[326]},
	{Value: intNumberValues[327]},
	{Value: intNumberValues[328]},
	{Value: intNumberValues[329]},
	{Value: intNumberValues[330]},
	{Value: intNumberValues[331]},
	{Value: intNumberValues[332]},
	{Value: intNumberValues[333]},
	{Value: intNumberValues[334]},
	{Value: intNumberValues[335]},
	{Value: intNumberValues[336]},
	{Value: intNumberValues[337]},
	{Value: intNumberValues[338]},
	{Value: intNumberValues[339]},
	{Value: intNumberValues[340]},
	{Value: intNumberValues[341]},
	{Value: intNumberValues[342]},
	{Value: intNumberValues[343]},
	{Value: intNumberValues[344]},
	{Value: intNumberValues[345]},
	{Value: intNumberValues[346]},
	{Value: intNumberValues[347]},
	{Value: intNumberValues[348]},
	{Value: intNumberValues[349]},
	{Value: intNumberValues[350]},
	{Value: intNumberValues[351]},
	{Value: intNumberValues[352]},
	{Value: intNumberValues[353]},
	{Value: intNumberValues[354]},
	{Value: intNumberValues[355]},
	{Value: intNumberValues[356]},
	{Value: intNumberValues[357]},
	{Value: intNumberValues[358]},
	{Value: intNumberValues[359]},
	{Value: intNumberValues[360]},
	{Value: intNumberValues[361]},
	{Value: intNumberValues[362]},
	{Value: intNumberValues[363]},
	{Value: intNumberValues[364]},
	{Value: intNumberValues[365]},
	{Value: intNumberValues[366]},
	{Value: intNumberValues[367]},
	{Value: intNumberValues[368]},
	{Value: intNumberValues[369]},
	{Value: intNumberValues[370]},
	{Value: intNumberValues[371]},
	{Value: intNumberValues[372]},
	{Value: intNumberValues[373]},
	{Value: intNumberValues[374]},
	{Value: intNumberValues[375]},
	{Value: intNumberValues[376]},
	{Value: intNumberValues[377]},
	{Value: intNumberValues[378]},
	{Value: intNumberValues[379]},
	{Value: intNumberValues[380]},
	{Value: intNumberValues[381]},
	{Value: intNumberValues[382]},
	{Value: intNumberValues[383]},
	{Value: intNumberValues[384]},
	{Value: intNumberValues[385]},
	{Value: intNumberValues[386]},
	{Value: intNumberValues[387]},
	{Value: intNumberValues[388]},
	{Value: intNumberValues[389]},
	{Value: intNumberValues[390]},
	{Value: intNumberValues[391]},
	{Value: intNumberValues[392]},
	{Value: intNumberValues[393]},
	{Value: intNumberValues[394]},
	{Value: intNumberValues[395]},
	{Value: intNumberValues[396]},
	{Value: intNumberValues[397]},
	{Value: intNumberValues[398]},
	{Value: intNumberValues[399]},
	{Value: intNumberValues[400]},
	{Value: intNumberValues[401]},
	{Value: intNumberValues[402]},
	{Value: intNumberValues[403]},
	{Value: intNumberValues[404]},
	{Value: intNumberValues[405]},
	{Value: intNumberValues[406]},
	{Value: intNumberValues[407]},
	{Value: intNumberValues[408]},
	{Value: intNumberValues[409]},
	{Value: intNumberValues[410]},
	{Value: intNumberValues[411]},
	{Value: intNumberValues[412]},
	{Value: intNumberValues[413]},
	{Value: intNumberValues[414]},
	{Value: intNumberValues[415]},
	{Value: intNumberValues[416]},
	{Value: intNumberValues[417]},
	{Value: intNumberValues[418]},
	{Value: intNumberValues[419]},
	{Value: intNumberValues[420]},
	{Value: intNumberValues[421]},
	{Value: intNumberValues[422]},
	{Value: intNumberValues[423]},
	{Value: intNumberValues[424]},
	{Value: intNumberValues[425]},
	{Value: intNumberValues[426]},
	{Value: intNumberValues[427]},
	{Value: intNumberValues[428]},
	{Value: intNumberValues[429]},
	{Value: intNumberValues[430]},
	{Value: intNumberValues[431]},
	{Value: intNumberValues[432]},
	{Value: intNumberValues[433]},
	{Value: intNumberValues[434]},
	{Value: intNumberValues[435]},
	{Value: intNumberValues[436]},
	{Value: intNumberValues[437]},
	{Value: intNumberValues[438]},
	{Value: intNumberValues[439]},
	{Value: intNumberValues[440]},
	{Value: intNumberValues[441]},
	{Value: intNumberValues[442]},
	{Value: intNumberValues[443]},
	{Value: intNumberValues[444]},
	{Value: intNumberValues[445]},
	{Value: intNumberValues[446]},
	{Value: intNumberValues[447]},
	{Value: intNumberValues[448]},
	{Value: intNumberValues[449]},
	{Value: intNumberValues[450]},
	{Value: intNumberValues[451]},
	{Value: intNumberValues[452]},
	{Value: intNumberValues[453]},
	{Value: intNumberValues[454]},
	{Value: intNumberValues[455]},
	{Value: intNumberValues[456]},
	{Value: intNumberValues[457]},
	{Value: intNumberValues[458]},
	{Value: intNumberValues[459]},
	{Value: intNumberValues[460]},
	{Value: intNumberValues[461]},
	{Value: intNumberValues[462]},
	{Value: intNumberValues[463]},
	{Value: intNumberValues[464]},
	{Value: intNumberValues[465]},
	{Value: intNumberValues[466]},
	{Value: intNumberValues[467]},
	{Value: intNumberValues[468]},
	{Value: intNumberValues[469]},
	{Value: intNumberValues[470]},
	{Value: intNumberValues[471]},
	{Value: intNumberValues[472]},
	{Value: intNumberValues[473]},
	{Value: intNumberValues[474]},
	{Value: intNumberValues[475]},
	{Value: intNumberValues[476]},
	{Value: intNumberValues[477]},
	{Value: intNumberValues[478]},
	{Value: intNumberValues[479]},
	{Value: intNumberValues[480]},
	{Value: intNumberValues[481]},
	{Value: intNumberValues[482]},
	{Value: intNumberValues[483]},
	{Value: intNumberValues[484]},
	{Value: intNumberValues[485]},
	{Value: intNumberValues[486]},
	{Value: intNumberValues[487]},
	{Value: intNumberValues[488]},
	{Value: intNumberValues[489]},
	{Value: intNumberValues[490]},
	{Value: intNumberValues[491]},
	{Value: intNumberValues[492]},
	{Value: intNumberValues[493]},
	{Value: intNumberValues[494]},
	{Value: intNumberValues[495]},
	{Value: intNumberValues[496]},
	{Value: intNumberValues[497]},
	{Value: intNumberValues[498]},
	{Value: intNumberValues[499]},
	{Value: intNumberValues[500]},
	{Value: intNumberValues[501]},
	{Value: intNumberValues[502]},
	{Value: intNumberValues[503]},
	{Value: intNumberValues[504]},
	{Value: intNumberValues[505]},
	{Value: intNumberValues[506]},
	{Value: intNumberValues[507]},
	{Value: intNumberValues[508]},
	{Value: intNumberValues[509]},
	{Value: intNumberValues[510]},
	{Value: intNumberValues[511]},
	{Value: intNumberValues[512]},
}
