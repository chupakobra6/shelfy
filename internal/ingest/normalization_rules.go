package ingest

import "strings"

type phraseRewrite struct {
	From string
	To   string
}

type canonicalPhraseRule struct {
	Match     string
	Canonical string
}

var draftFillerPrefixes = []string{
	"слушай",
	"короче",
	"вообще",
	"пожалуйста",
	"окей",
	"ок",
	"такс",
	"тэк",
	"так",
	"ну",
	"ты экс",
	"в силе",
	"бессилием",
	"у меня тут",
	"у меня",
	"меня",
	"привет",
	"добрый вечер",
	"доброе утро",
	"добрый день",
	"алиса",
	"джой",
	"сири",
	"салют",
	"афина",
	"ассистент",
	"помоги",
	"можешь",
}

var draftActionPrefixes = []string{
	"я хочу заказать",
	"я хотела бы заказать",
	"хотел бы заказать",
	"хотела бы заказать",
	"хотя бы заказать",
	"хочу заказать",
	"можешь заказать",
	"можешь мне заказать",
	"закажи мне",
	"закажи на дом",
	"заказать на дом",
	"заказать",
	"мне нужна",
	"мне нужен",
	"мне нужно",
	"мне нужны",
	"мне нужно записать",
	"нужно записать",
	"надо записать",
	"нужно добавить",
	"надо добавить",
	"нужно купить",
	"надо купить",
	"нужно взять",
	"надо взять",
	"добавь в корзину",
	"добавить в корзину",
	"добавь",
	"добавить",
	"купи",
	"купить",
	"закажи",
	"запиши",
	"записать",
	"взять",
	"нужна",
	"нужен",
	"нужно",
	"нужны",
	"надо",
}

var draftLeadingNoisePhrases = []string{
	"из магазина",
	"из ленты",
	"из монетки",
	"с доставкой",
	"на дом",
	"домой",
	"с бесплатной доставкой",
	"срочно",
	"как можно быстрее",
}

var draftTrailingCutPhrases = []string{
	" с доставкой ",
	" на дом ",
	" домой ",
	" из магазина ",
	" из ленты ",
	" из монетки ",
	" как можно быстрее ",
	" через полчаса ",
	" через час ",
	" к завтраку ",
	" срок годности ",
	" сок годности ",
	" закажи пожалуйста ",
	" со вкусом ",
	" в корзину ",
}

var genericContainerNames = map[string]struct{}{
	"продукты": {},
	"покупки":  {},
	"еда":      {},
}

var storeNoiseTokens = map[string]struct{}{
	"монетка":     {},
	"лента":       {},
	"магнит":      {},
	"пятерочка":   {},
	"перекресток": {},
	"ашан":        {},
	"окей":        {},
	"вкусвилл":    {},
}

var quantityNoiseTokens = map[string]struct{}{
	"один": {}, "одна": {}, "одно": {}, "одну": {},
	"два": {}, "две": {}, "три": {}, "четыре": {}, "пять": {},
	"шесть": {}, "семь": {}, "восемь": {}, "девять": {}, "десять": {},
	"ноль": {}, "пол": {}, "полтора": {}, "полкило": {}, "поллитра": {},
	"полкилограмма": {},
	"половиной":     {}, "пятьсот": {}, "семьсот": {}, "девятьсот": {},
	"килограмм": {}, "килограмма": {}, "килограммов": {},
	"грамм": {}, "грамма": {}, "граммов": {},
	"литр": {}, "литра": {}, "литров": {},
	"литровую": {}, "литровой": {},
	"миллилитров": {}, "миллилитра": {}, "миллилитр": {},
	"штука": {}, "штуки": {}, "штук": {},
	"бутылка": {}, "бутылки": {}, "бутылку": {},
	"упаковка": {}, "упаковки": {}, "упаковку": {},
	"пакет": {}, "пакета": {}, "пакетов": {},
	"пакетик": {}, "пакетика": {}, "пакетиках": {},
	"пачка": {}, "пачки": {}, "пачку": {},
	"банка": {}, "банки": {}, "банку": {},
	"ведро": {}, "буханка": {}, "булка": {}, "булки": {},
	"десяток":  {},
	"двадцать": {},
	"тридцать": {},
	"сорок":    {},
}

var packagingNoiseTokens = map[string]struct{}{
	"стекло": {}, "стеклянная": {}, "стеклянный": {}, "стеклянную": {},
	"бутылке": {}, "бутылочный": {}, "вафельном": {}, "стаканчике": {},
	"упаковке": {}, "упаковку": {}, "ведро": {},
}

var fatnessNoiseTokens = map[string]struct{}{
	"процент": {}, "процента": {}, "процентов": {},
	"жирность": {}, "жирности": {}, "жирностью": {},
	"процентной": {}, "процентное": {}, "процентный": {},
	"шестипроцентное": {}, "четырехпроцентная": {}, "обезжиренное": {},
}

var deliveryNoiseTokens = map[string]struct{}{
	"доставка": {}, "доставкой": {}, "доставку": {},
	"дом": {}, "домой": {}, "срочно": {}, "быстрее": {},
	"привезите": {}, "привези": {}, "бесплатной": {},
	"улица": {}, "подъезд": {}, "квартира": {},
}

var timeOnlyNoiseTokens = map[string]struct{}{
	"полчаса": {}, "час": {}, "часа": {}, "часов": {},
	"утро": {}, "утром": {}, "вечер": {}, "вечером": {},
	"ночью": {}, "ночь": {}, "восемь": {}, "девять": {},
}

var draftTrailingCutTokens = func() map[string]struct{} {
	out := map[string]struct{}{}
	for _, group := range []map[string]struct{}{quantityNoiseTokens, packagingNoiseTokens, fatnessNoiseTokens, deliveryNoiseTokens} {
		for key := range group {
			out[key] = struct{}{}
		}
	}
	for _, token := range []string{"в", "во", "со", "с", "вкусом", "ультрапастеризованное", "ультрапастеризованный", "негазированная", "негазированный"} {
		out[token] = struct{}{}
	}
	for _, token := range []string{"закажи", "заказать", "пожалуйста"} {
		out[token] = struct{}{}
	}
	return out
}()

var productCanonicalLeadToken = map[string]string{
	"курицу":    "курица",
	"колбасу":   "колбаса",
	"ряженку":   "ряженка",
	"сметану":   "сметана",
	"воду":      "вода",
	"молока":    "молоко",
	"масло":     "масло",
	"хлеба":     "хлеб",
	"бананов":   "бананы",
	"помидоров": "помидоры",
	"помидор":   "помидоры",
}

var canonicalProductPhraseRules = []canonicalPhraseRule{
	{Match: "молоко домик в деревне", Canonical: "молоко домик в деревне"},
	{Match: "домик в деревне", Canonical: "молоко домик в деревне"},
	{Match: "молоко простоквашино", Canonical: "молоко простоквашино"},
	{Match: "молока простоквашино", Canonical: "молоко простоквашино"},
	{Match: "молоко ильинское", Canonical: "молоко ильинское"},
	{Match: "молоко ильинская", Canonical: "молоко ильинское"},
	{Match: "молоко рогачевское", Canonical: "молоко рогачевское"},
	{Match: "молоко рогачев ская", Canonical: "молоко рогачевское"},
	{Match: "молоко пармалат", Canonical: "молоко пармалат"},
	{Match: "молоко коровка из кореновки", Canonical: "молоко коровка из кореновки"},
	{Match: "ряженка кубанская буренка", Canonical: "ряженка кубанская буренка"},
	{Match: "чай тесс зеленый", Canonical: "чай тесс зеленый"},
	{Match: "черный чай принцесса нури", Canonical: "черный чай принцесса нури"},
	{Match: "чай ахмад ти инглиш брекфаст", Canonical: "чай ахмад ти инглиш брекфаст"},
	{Match: "чай принцесса нури", Canonical: "чай принцесса нури"},
	{Match: "чай лисма", Canonical: "чай лисма"},
	{Match: "чай гринфилд", Canonical: "чай гринфилд"},
	{Match: "чай акбар", Canonical: "чай акбар"},
	{Match: "зеленый рассыпной чай соус", Canonical: "зеленый рассыпной чай"},
	{Match: "зеленый рассыпной чай саусеп", Canonical: "зеленый рассыпной чай"},
	{Match: "вода святой источник", Canonical: "вода святой источник"},
	{Match: "вода родниковая", Canonical: "вода родниковая"},
	{Match: "пельмени мираторг", Canonical: "пельмени мираторг"},
	{Match: "пельмени лукович", Canonical: "пельмени лукович"},
	{Match: "пельмени сибирская коллекция домашние", Canonical: "пельмени сибирская коллекция домашние"},
	{Match: "пельмени стародворье сливочные", Canonical: "пельмени стародворье сливочные"},
	{Match: "пельмени фроловские", Canonical: "пельмени фроловские"},
	{Match: "пельмени цезарь классика", Canonical: "пельмени цезарь классика"},
	{Match: "пюре фрутоняня яблоко", Canonical: "пюре фрутоняня яблоко"},
	{Match: "бекон велком", Canonical: "бекон велком"},
	{Match: "черный уральский хлеб", Canonical: "черный уральский хлеб"},
	{Match: "черного уральского хлеба", Canonical: "черный уральский хлеб"},
	{Match: "хлеб крестьянский", Canonical: "хлеб крестьянский"},
	{Match: "белый хлеб", Canonical: "белый хлеб"},
	{Match: "колбаса вязанка", Canonical: "колбаса вязанка"},
	{Match: "колбасу вязанка", Canonical: "колбаса вязанка"},
	{Match: "сосиски клинские молочные", Canonical: "сосиски клинские молочные"},
	{Match: "сосиски родионовские", Canonical: "сосиски родионовские"},
	{Match: "сосиски родионов ские", Canonical: "сосиски родионовские"},
	{Match: "котлеты петелинка сливочные", Canonical: "котлеты петелинка сливочные"},
	{Match: "мороженое белочка", Canonical: "мороженое белочка"},
	{Match: "мороженое село зеленое пломбир", Canonical: "мороженое село зеленое пломбир"},
	{Match: "мороженое филевское пломбир", Canonical: "мороженое филевское пломбир"},
	{Match: "мороженое чистая линия ванильное", Canonical: "мороженое чистая линия ванильное"},
	{Match: "чистая линия ванильное мороженое", Canonical: "мороженое чистая линия ванильное"},
	{Match: "мороженое снеговик", Canonical: "мороженое снеговик"},
	{Match: "пирожные тирамису", Canonical: "пирожные тирамису"},
	{Match: "помидоры черри", Canonical: "помидоры черри"},
	{Match: "печенье с малиновой начинкой", Canonical: "печенье с малиновой начинкой"},
	{Match: "масло подсолнечное золотая семечка", Canonical: "масло подсолнечное золотая семечка"},
	{Match: "молоко большая кружка", Canonical: "молоко большая кружка"},
	{Match: "колбаса черкашина", Canonical: "колбаса черкашина"},
	{Match: "колбасу черкашина", Canonical: "колбаса черкашина"},
	{Match: "пельмени ложкарев", Canonical: "пельмени ложкарев"},
	{Match: "ложкарев пельмени", Canonical: "пельмени ложкарев"},
	{Match: "ложкарев полкилограмма пельмени", Canonical: "пельмени ложкарев"},
	{Match: "чипсы принглс", Canonical: "чипсы принглс"},
	{Match: "чипсы лейс", Canonical: "чипсы лейс"},
	{Match: "сок яблочный", Canonical: "сок яблочный"},
	{Match: "сок виноградный любимый", Canonical: "сок виноградный любимый"},
	{Match: "масло", Canonical: "масло"},
}

var voiceTranscriptPhraseRewrites = []phraseRewrite{
	{From: "он то", To: "до"},
	{From: "а то", To: "до"},
	{From: "ты экс", To: ""},
	{From: "такс", To: "так"},
	{From: "тэк", To: "так"},
	{From: "сок годности", To: "срок годности"},
	{From: "млако", To: "молоко"},
	{From: "рогачев ская", To: "рогачевское"},
	{From: "родионов ские", To: "родионовские"},
	{From: "фра орловские", To: "фроловские"},
	{From: "ильинская", To: "ильинское"},
	{From: "зеленые рассыпной", To: "зеленый рассыпной"},
	{From: "чай соуса", To: "чай соус"},
	{From: "ложкарев полкилограмма пельмени", To: "пельмени ложкарев"},
	{From: "шести процентной", To: "шестипроцентное"},
	{From: "две штуки", To: "две штуки"},
}

var draftNamePhraseRewrites = []phraseRewrite{
	{From: "фирмы", To: ""},
	{From: "бренд", To: ""},
	{From: "бренда", To: ""},
	{From: "с названием", To: ""},
	{From: "со вкусом", To: ""},
	{From: "со вкусом сыра", To: ""},
	{From: "с саусеп", To: ""},
	{From: "с бесплатной", To: ""},
	{From: "ложкарев полкилограмма пельмени", To: "ложкарев пельмени"},
}

var foodLexiconPhrases = uniqueStrings(append([]string{
	"молоко",
	"молока",
	"кефир",
	"кефира",
	"ряженка",
	"ряженку",
	"ряженки",
	"сметана",
	"сметану",
	"творог",
	"сыр",
	"сыра",
	"курица",
	"курицу",
	"салат",
	"бананы",
	"бананов",
	"сливки",
	"масло",
	"йогурт",
	"йогурта",
	"хумус",
	"колбаса",
	"колбасу",
	"фарш",
	"яйца",
	"яиц",
	"хлеб",
	"хлеба",
	"соус",
	"зефир",
	"чай",
	"пельмени",
	"вода",
	"воду",
	"мороженое",
	"бекон",
	"пюре",
	"пирожные",
	"помидоры",
	"помидоров",
	"яблоки",
	"яблок",
	"кокосовое молоко",
	"соевое молоко",
	"сосиски",
	"котлеты",
	"чипсы",
	"сок",
	"печенье",
	"яблоки",
	"огурцы",
	"перец",
	"шоколад",
	"попкорн",
	"мороженое",
	"печень",
	"картофель",
	"оливки",
	"маслины",
}, canonicalOutputs(canonicalProductPhraseRules)...))

var foodLexiconTokens = buildFoodLexiconTokens(foodLexiconPhrases, []string{
	"черный", "зеленый", "питьевой", "домик", "деревне", "святой", "источник",
	"простоквашино", "ильинское", "рогачевское", "пармалат", "мираторг", "лукович",
	"вязанка", "велком", "фрутоняня", "тирамису", "черри", "агуша", "петелинка",
	"принглс", "лейс", "гринфилд", "акбар", "лисма", "ахмад", "нури",
})

var rejectIntentPhrases = []string{
	"напомни", "напомнить", "встреча", "встречу", "список", "убрать", "уборка",
	"позвони", "позвонить", "созвон", "митинг", "задача", "задачи", "дело", "дела",
	"окулисту", "массаж", "будильник", "телеканал", "сериал", "кино", "видео", "трек",
	"передачу", "телевизоре", "тв", "смотрешке", "ютюбе", "найди", "покажи", "включай",
	"запиши", "установить", "поставь", "время", "сколько", "режим", "светомузыку",
	"замените", "заменить", "пролили", "пролил", "корм", "китикет", "вискас", "пурина",
	"оладьи", "яичницу", "тосты",
}

var voiceDateSignalTokens = []string{
	"сегодня", "завтра", "послезавтра", "до", "к", "на", "в", "во",
	"пн", "вт", "ср", "чт", "пт", "сб", "вс",
	"понедельник", "вторник", "среда", "четверг", "пятница", "суббота", "воскресенье",
	"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек",
	"день", "дня", "дней", "неделю", "недели", "недель", "месяц", "месяца", "месяцев",
}

var voiceNoCorrectionTokens = mergeStringSets(
	keysOfMap(quantityNoiseTokens),
	keysOfMap(packagingNoiseTokens),
	keysOfMap(fatnessNoiseTokens),
	keysOfMap(deliveryNoiseTokens),
	keysOfMap(timeOnlyNoiseTokens),
	rejectIntentPhrases,
	voiceDateSignalTokens,
	[]string{"пожалуйста", "закажи", "заказать", "купи", "купить", "добавь", "добавить", "мне", "нужен", "нужна", "нужно", "нужны", "слушай", "привет", "добрый", "вечер"},
)

func canonicalOutputs(rules []canonicalPhraseRule) []string {
	out := make([]string, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rule.Canonical)
	}
	return out
}

func buildFoodLexiconTokens(phrases []string, extra []string) []string {
	set := map[string]struct{}{}
	for _, phrase := range phrases {
		for _, token := range strings.Fields(phrase) {
			set[token] = struct{}{}
		}
	}
	for _, token := range extra {
		set[token] = struct{}{}
	}
	return keysOfMap(set)
}

func uniqueStrings(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return keysOfMap(set)
}

func mergeStringSets(groups ...[]string) []string {
	set := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			set[value] = struct{}{}
		}
	}
	return keysOfMap(set)
}

func keysOfMap[T any](set map[string]T) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return out
}

func rewriteNormalizedPhrases(input string, rewrites []phraseRewrite) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	value := " " + normalizedPolicyText(input) + " "
	for _, rewrite := range rewrites {
		value = strings.ReplaceAll(value, " "+rewrite.From+" ", " "+rewrite.To+" ")
	}
	return strings.TrimSpace(strings.Join(strings.Fields(value), " "))
}

func canonicalProductPhrase(input string) (string, bool) {
	normalized := " " + normalizedPolicyText(input) + " "
	best := ""
	for _, rule := range canonicalProductPhraseRules {
		match := " " + rule.Match + " "
		if !strings.Contains(normalized, match) {
			continue
		}
		if len(rule.Match) > len(best) {
			best = rule.Canonical
		}
	}
	return best, best != ""
}

func containsAnyToken(input string, tokens map[string]struct{}) bool {
	normalized := normalizedPolicyText(input)
	if normalized == "" {
		return false
	}
	for _, token := range strings.Fields(normalized) {
		if _, ok := tokens[token]; ok {
			return true
		}
	}
	return false
}

func isGenericContainerName(input string) bool {
	normalized := normalizedPolicyText(input)
	_, ok := genericContainerNames[normalized]
	return ok
}
