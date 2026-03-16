package wordgen

import (
	"math"
	"math/rand"
	"strings"
)

// vocabulary is a ~5000 common English word list embedded as a constant.
var vocabulary = strings.Fields(vocabRaw)

const vocabRaw = `the be to of and a in that have it for not on with he as you do at
this but his by from they we say her she or an will my one all would there their what
so up out if about who get which go me when make can like time no just him know take
people into year your good some could them see other than then now look only come its over
think also back after use two how our work first well way even new want because any these
give day most us great between need large often hand high place hold no world every found still
learn plant cover food sun four between state keep eye never last let thought city tree cross farm
hard start might story saw far sea draw left late run don't while press close night real life few
north open seem together next white children begin got walk example ease paper group always music
those both mark often letter until mile river car feet care second enough plain girl usual young
ready above ever red list though feel talk bird soon body dog family direct pose leave song measure
door product black short numeral energy force town fine drive fell cry dark machine note wait plan
figure star box noun field rest correct able pound done beauty drive stood contain front teach week
final gave green oh quick develop ocean warm free minute strong special mind behind clear tail produce
fact street inch multiply nothing course stay wheel full force blue object decide surface deep moon island
foot system busy test record boat common gold possible plane stead dry wonder laugh thousand ago ran check
game shape equate hot miss brought heat snow tire bring yes distant fill east paint language among grand
ball yet wave drop heart am present heavy dance engine position arm wide sail material size vary settle
speak weight general ice matter circle pair include divide syllable felt perhaps pick sudden count square reason
length represent art subject region energy hunt probable bed brother egg ride cell believe fraction forest sit
race window store summer train sleep prove lone leg exercise wall catch mount wish sky board joy winter
sat written wild instrument kept glass grass cow job edge sign visit past soft fun bright gas weather
month million bear finish happy hope flower clothe strange gone jump baby eight village meet root buy raise
solve metal whether push seven paragraph third shall held hair describe cook floor either result burn hill safe
cat century consider type law bit coast copy phrase silent tall sand soil roll temperature finger industry value
fight lie beat excite natural view plain queen gas check game pain broad size vary settle dark perhaps
object flat twenty skin smile crease hole trade melody trip office receive row mouth exact symbol die least
trouble shout except wrote seed tone join suggest clean break lady yard rise bad blow oil blood touch
grew cent mix team wire cost lost brown wear garden equal sent choose fell fit flow fair bank
collect save control decimal gentle woman captain practice separate difficult doctor please protect noon whose locate ring
character insect caught period indicate radio spoke atom human history effect electric expect crop modern element hit
student corner party supply bone rail imagine provide agree thus capital won't chair danger fruit rich thick soldier
process operate guess necessary sharp wing create neighbor wash bat rather crowd corn compare poem string bell depend
meat rub tube famous dollar stream fear sight thin triangle planet hurry chief colony clock mine tie enter
major fresh search send yellow gun allow print dead spot desert suit current lift rose continue block chart
hat sell success company subtract event particular deal swim term opposite wife shoe shoulder spread arrange camp invent
cotton born determine quart nine truck noise level chance gather shop stretch throw shine property column molecule select
wrong gray repeat require broad prepare salt nose plural anger claim capital diet gentle quiet guess fact valley
total noisy bring watch shell dry above either result burn hill safe cat century consider type law bit
consonant nation dictionary milk speed method organ pay age section dress cloud surprise quiet stone tiny climb cool
design poor lot experiment bottom key iron single stick flat twenty skin smile crease hole trade melody trip
office receive row mouth exact symbol die least trouble shout except wrote seed tone join suggest clean break
track parent shade division sheet substance favor connect post spend chord fat glad original share station dad bread
charge proper bar offer segment slave duck instant market degree populate chick dear enemy reply drink occur support
speech nature range steam motion path liquid log meant quotient teeth shell neck`

// TagPhrases is a list of short English phrases embedded in a fraction of entries
// to enable FTS5 benchmarking. Each phrase is realistic and searchable.
var TagPhrases = []string{
	"meeting notes",
	"action items",
	"deployment plan",
	"sprint planning",
	"technical debt",
	"code review",
	"release notes",
	"bug report",
	"feature request",
	"architecture decision",
	"performance review",
	"project kickoff",
	"status update",
	"retrospective notes",
	"quarterly goals",
	"incident report",
	"design review",
	"onboarding checklist",
	"team sync",
	"product roadmap",
	"security audit",
	"capacity planning",
	"risk assessment",
	"knowledge transfer",
	"post mortem",
}

// GenerateWithPhrase generates content like Generate but inserts phrase at a
// random word boundary within the generated text.
func GenerateWithPhrase(rng *rand.Rand, phrase string) string {
	wordCount := lognormal(rng, 4.0, 1.5)
	if wordCount < 20 {
		wordCount = 20
	}
	if wordCount > 20000 {
		wordCount = 20000
	}
	base := generateMarkdown(rng, wordCount)
	// Collect space positions as candidate insertion points.
	var spaces []int
	for i, c := range base {
		if c == ' ' {
			spaces = append(spaces, i)
		}
	}
	if len(spaces) == 0 {
		return base[:len(base)-1] + " " + phrase + "\n"
	}
	pos := spaces[rng.Intn(len(spaces))]
	return base[:pos] + " " + phrase + base[pos:]
}

// Generate returns a markdown string with approximately `words` words.
// It uses log-normal distribution: mu=4.0, sigma=1.5, clamped to [5, 20000].
func Generate(rng *rand.Rand) string {
	wordCount := lognormal(rng, 4.0, 1.5)
	if wordCount < 5 {
		wordCount = 5
	}
	if wordCount > 20000 {
		wordCount = 20000
	}
	return generateMarkdown(rng, wordCount)
}

// GenerateN generates content with exactly n words.
func GenerateN(rng *rand.Rand, n int) string {
	return generateMarkdown(rng, n)
}

func lognormal(rng *rand.Rand, mu, sigma float64) int {
	z := rng.NormFloat64()
	return int(math.Round(math.Exp(mu + sigma*z)))
}

func generateMarkdown(rng *rand.Rand, wordCount int) string {
	vlen := len(vocabulary)
	var sb strings.Builder
	sb.Grow(wordCount * 6)

	written := 0
	paragraphTarget := 40 + rng.Intn(41) // 40–80 words per paragraph
	inParagraph := 0
	firstParagraph := true

	for written < wordCount {
		// Occasionally add a heading (roughly every 150–300 words, not at start)
		if !firstParagraph && inParagraph == 0 && rng.Intn(200) < 1 {
			heading := "## "
			sb.WriteString(heading)
			// heading has 3–6 words
			hw := 3 + rng.Intn(4)
			for i := 0; i < hw && written < wordCount; i++ {
				if i > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(capitalize(vocabulary[rng.Intn(vlen)]))
				written++
			}
			sb.WriteString("\n\n")
			paragraphTarget = 40 + rng.Intn(41)
			continue
		}

		word := vocabulary[rng.Intn(vlen)]
		if inParagraph == 0 {
			if !firstParagraph {
				sb.WriteString("\n\n")
			}
			word = capitalize(word)
			firstParagraph = false
		} else {
			sb.WriteByte(' ')
		}
		sb.WriteString(word)
		written++
		inParagraph++

		if inParagraph >= paragraphTarget {
			sb.WriteByte('.')
			inParagraph = 0
			paragraphTarget = 40 + rng.Intn(41)
		}
	}

	// Close last paragraph
	if inParagraph > 0 {
		sb.WriteByte('.')
	}
	sb.WriteByte('\n')
	return sb.String()
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 32
	}
	return string(b)
}
