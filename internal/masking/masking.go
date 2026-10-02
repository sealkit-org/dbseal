package masking

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"

	"github.com/sealkit/dbseal/pkg/extension"
)

// DefaultFixedSalt ensures OSS deterministic hashing produces consistent maps across tables.
const DefaultFixedSalt = "dbseal_oss_default_deterministic_salt_v1"

func init() {
	extension.SetBaseMasker(ApplyRule)
}

// Engine manages column masking execution.
type Engine struct {
	mu           sync.RWMutex
	customRule   map[string]func(val any) any
	tableColumns map[string]map[string]string // table -> column -> rule
}

func NewEngine(tableRules map[string]map[string]string) *Engine {
	return &Engine{
		tableColumns: tableRules,
		customRule:   make(map[string]func(val any) any),
	}
}

// MaskColumn masks an individual column value based on table and column name.
func (e *Engine) MaskColumn(table, column string, val any) (any, error) {
	if val == nil {
		return nil, nil
	}

	cols, ok := e.tableColumns[table]
	if !ok {
		return val, nil // No masking configured for this table
	}

	rule, ok := cols[column]
	if !ok {
		return val, nil // No masking configured for this column
	}

	return ApplyRule(val, rule)
}

// ApplyRule evaluates a masking rule expression on a value.
func ApplyRule(val any, rule string) (any, error) {
	if val == nil {
		return nil, nil
	}

	rule = strings.TrimSpace(rule)

	// Check custom extension maskers (e.g. Pro JSONPath, dynamic HMAC)
	ruleName := rule
	ruleParam := ""
	if idx := strings.Index(rule, ":"); idx != -1 {
		ruleName = rule[:idx]
		ruleParam = rule[idx+1:]
	}
	if m, ok := extension.GetMasker(ruleName); ok {
		return m.Mask(val, ruleParam)
	}

	switch {
	case rule == "null":
		return nil, nil

	case strings.HasPrefix(rule, "constant."):
		return strings.TrimPrefix(rule, "constant."), nil

	case rule == "faker.name":
		return FakeName(), nil

	case rule == "faker.email":
		return FakeEmail(), nil

	case rule == "faker.phone_number":
		return FakePhoneNumber(), nil

	case rule == "faker.address":
		return FakeAddress(), nil

	case rule == "faker.ipv4":
		return FakeIPv4(), nil

	case strings.HasPrefix(rule, "mask.keep_first_last"):
		// Format: mask.keep_first_last(3, 4)
		return MaskKeepFirstLast(fmt.Sprintf("%v", val), 3, 4), nil

	case rule == "hash.sha256" || rule == "deterministic.hash":
		return DeterministicHash(val, DefaultFixedSalt), nil

	default:
		// Fallback: if unknown rule, check if it's constant
		return val, nil
	}
}

// --- Fake data generators ---

var (
	firstNames = []string{"Alex", "Jordan", "Taylor", "Morgan", "Sam", "Chris", "Pat", "Robin", "Casey", "Riley", "张", "李", "王", "赵", "陈", "刘", "杨", "黄"}
	lastNames  = []string{"Smith", "Johnson", "Williams", "Brown", "Jones", "Miller", "Davis", "伟", "芳", "娜", "敏", "静", "丽", "强", "磊", "洋", "勇"}
	domains    = []string{"example.com", "testmail.org", "devmail.net", "sample.io"}
	cities     = []string{"New York, NY", "San Francisco, CA", "Seattle, WA", "Austin, TX", "London, UK", "北京市朝阳区", "上海市浦东新区", "深圳市南山区", "杭州市西湖区"}
)

func FakeName() string {
	fn := firstNames[rand.IntN(len(firstNames))]
	ln := lastNames[rand.IntN(len(lastNames))]
	return fn + " " + ln
}

func FakeEmail() string {
	user := strings.ToLower(firstNames[rand.IntN(len(firstNames))]) + fmt.Sprintf("%04d", rand.IntN(10000))
	domain := domains[rand.IntN(len(domains))]
	return user + "@" + domain
}

func FakePhoneNumber() string {
	prefixes := []string{"138", "139", "150", "158", "186", "188"}
	p := prefixes[rand.IntN(len(prefixes))]
	return fmt.Sprintf("%s%08d", p, rand.IntN(100000000))
}

func FakeAddress() string {
	city := cities[rand.IntN(len(cities))]
	streetNum := rand.IntN(900) + 100
	return fmt.Sprintf("%d %s", streetNum, city)
}

func FakeIPv4() string {
	return fmt.Sprintf("192.168.%d.%d", rand.IntN(254)+1, rand.IntN(254)+1)
}

func MaskKeepFirstLast(s string, keepHead, keepTail int) string {
	runes := []rune(s)
	total := len(runes)
	if total <= keepHead+keepTail {
		return strings.Repeat("*", total)
	}

	head := string(runes[:keepHead])
	tail := string(runes[total-keepTail:])
	maskedCount := total - keepHead - keepTail
	if maskedCount > 6 {
		maskedCount = 4
	}
	return head + strings.Repeat("*", maskedCount) + tail
}

// DeterministicHash provides a consistent hash value across tables preserving relational integrity.
func DeterministicHash(val any, salt string) any {
	str := fmt.Sprintf("%s:%v", salt, val)
	h := sha256.Sum256([]byte(str))
	hexStr := hex.EncodeToString(h[:])

	switch val.(type) {
	case int, int64, int32:
		// Convert first 8 bytes of hash into positive int64 to preserve integer type
		num, err := strconv.ParseInt(hexStr[:8], 16, 64)
		if err == nil && num > 0 {
			return num
		}
		return int64(100000 + rand.IntN(900000))
	default:
		// If string contains @, format like an email hash
		if strings.Contains(fmt.Sprintf("%v", val), "@") {
			return hexStr[:12] + "@masked.example.com"
		}
		return hexStr[:16]
	}
}
