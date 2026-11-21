package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// К12-и4: имя последнего ранга не уходит гостю, пока ранг не взят; взявшему — на трёх языках.
func TestSecretRankHidden(t *testing.T) {
	last := accountRanks[len(accountRanks)-1]
	if secretRankNames["ru"] != last.Name {
		t.Fatalf("русское имя тайного ранга %q расходится с таблицей %q", secretRankNames["ru"], last.Name)
	}
	for _, lvl := range []int{1, last.Level - 1} {
		h := rankShown(last, lvl, gin.H{"level": last.Level})
		b, _ := json.Marshal(h)
		for _, n := range secretRankNames {
			if strings.Contains(string(b), n) {
				t.Errorf("ранг %d: имя %q утекло: %s", lvl, n, b)
			}
		}
		if h["name"] != secretRankMask || h["secret"] != true {
			t.Errorf("ранг %d: ждали «***» и secret: %v", lvl, h)
		}
	}
	h := rankShown(last, last.Level, gin.H{})
	if h["name"] != last.Name || h["secret"] != nil || h["names"].(map[string]string)["en"] != "Immortal" {
		t.Errorf("взятый ранг: %v", h)
	}
	if h := rankShown(accountRanks[5], 1, gin.H{}); h["name"] != "Легенда" || h["secret"] != nil || h["names"] != nil {
		t.Errorf("обычный ранг: %v", h)
	}
}
