package knowledge

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/vibeasik/vibeasik/pkg/models"
)

// Jaring pengalaman antarpegawai. Pengalaman pribadi yang mulai terbukti ditawarkan ke pegawai lain yang bekerja di
// bidang yang sama. Penerima memakainya lewat rujukan ke butir yang sama (KnowledgeAdoption, bukan salinan), sehingga
// bukti dari pekerjaannya ikut menguatkan butir itu. Setelah masa coba, penerima mempertahankan atau membuangnya.
// Pengalaman yang terbukti di banyak pegawai dan proyek naik menjadi patokan kantor (promote).

// Ambang berbagi dan masa coba.
const (
	shareMinHelpful      = 2                   // bukti kuat ✓ minimal sebelum ditawarkan
	shareMinAge          = 7 * 24 * time.Hour  // umur butir minimal
	shareActiveWindow    = 30 * 24 * time.Hour // penerima harus bekerja di domain itu dalam jendela ini
	shareMaxPerWeek      = 5                   // tawaran baru per penerima per 7 hari
	shareMaxPerNodeRound = 3                   // penerima baru per butir per putaran kurasi
	trialMaxUses         = 5                   // disodorkan sekian kali tanpa dikutip → dibuang
	trialMaxAge          = 21 * 24 * time.Hour // masa coba terpanjang
)

// projectSpecificRe mengenali pengalaman yang terikat satu aplikasi: nama berkas kode, nomor baris "(L428)", todo
// bernomor, atau path rumah. Pengalaman seperti itu tetap milik pegawainya, tetapi tidak ditawarkan ke rekan dan
// tidak dipromosikan menjadi patokan.
var projectSpecificRe = regexp.MustCompile(`(?i)\b[\w.-]+\.(go|vue|tsx?|jsx?|dart|php|py|rb|kt|swift|java|sql)\b|\(L\d+\)|\btodo #\d+|/home/|/Users/`)

// ProjectSpecific: butir ini menyebut berkas/baris/todo satu aplikasi.
func ProjectSpecific(n models.KnowledgeNode) bool {
	return projectSpecificRe.MatchString(n.Title + "\n" + n.Content)
}

// adoptedBy: butir pengalaman pegawai lain yang sedang dicoba atau dipertahankan pegawai ini.
func adoptedBy(db *gorm.DB, employeeID uint) map[uint]bool {
	if db == nil || employeeID == 0 {
		return nil
	}
	var ids []uint
	db.Model(&models.KnowledgeAdoption{}).Where("employee_id = ? AND status IN ?", employeeID,
		[]string{models.KnowledgeAdoptionTrial, models.KnowledgeAdoptionKept}).Pluck("node_id", &ids)
	if len(ids) == 0 {
		return nil
	}
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// recordAdoptionUses menambah hitungan "disodorkan" untuk butir adopsi yang ikut masuk prompt pegawai.
func recordAdoptionUses(db *gorm.DB, employeeID uint, ids []uint) {
	if employeeID == 0 || len(ids) == 0 {
		return
	}
	db.Model(&models.KnowledgeAdoption{}).Where("employee_id = ? AND node_id IN ? AND status IN ?", employeeID, ids,
		[]string{models.KnowledgeAdoptionTrial, models.KnowledgeAdoptionKept}).UpdateColumn("uses", gorm.Expr("uses + 1"))
}

// SettleAdoptions mencatat hasil satu todo penerima pada butir adopsinya: dikutip (dan lulus → ✓), ditandai keliru
// (→ ✗ lalu dibuang), atau ditandai tidak cocok (→ dibuang tanpa menyalahkan isinya).
func SettleAdoptions(db *gorm.DB, employeeID uint, passed bool, cited, wrong, unfit []uint) {
	if db == nil || employeeID == 0 {
		return
	}
	active := []string{models.KnowledgeAdoptionTrial, models.KnowledgeAdoptionKept}
	if len(cited) > 0 {
		up := map[string]interface{}{"cited": gorm.Expr("cited + 1")}
		if passed {
			up["helpful"] = gorm.Expr("helpful + 1")
		}
		db.Model(&models.KnowledgeAdoption{}).Where("employee_id = ? AND node_id IN ? AND status IN ?", employeeID, cited, active).Updates(up)
	}
	if len(wrong) > 0 {
		db.Model(&models.KnowledgeAdoption{}).Where("employee_id = ? AND node_id IN ? AND status IN ?", employeeID, wrong, active).
			UpdateColumn("harmful", gorm.Expr("harmful + 1"))
		DropAdoption(db, employeeID, wrong, "ditandai keliru oleh penerima")
	}
	if len(unfit) > 0 {
		DropAdoption(db, employeeID, unfit, "tidak cocok untuk penerima")
	}
}

// DropAdoption: penerima membuang butir adopsi (tidak ditawarkan lagi kepadanya). Mengembalikan jumlah yang dibuang.
func DropAdoption(db *gorm.DB, employeeID uint, nodeIDs []uint, reason string) int {
	if db == nil || employeeID == 0 || len(nodeIDs) == 0 {
		return 0
	}
	now := time.Now()
	res := db.Model(&models.KnowledgeAdoption{}).Where("employee_id = ? AND node_id IN ? AND status <> ?", employeeID, nodeIDs, models.KnowledgeAdoptionDropped).
		Updates(map[string]interface{}{"status": models.KnowledgeAdoptionDropped, "reason": clip(reason, 250), "decided_at": &now})
	if res.RowsAffected > 0 {
		invalidate()
	}
	return int(res.RowsAffected)
}

// offerShares menawarkan pengalaman pribadi yang mulai terbukti (✓ ≥ 2, tanpa ✗, umur ≥ 7 hari, berdomain) kepada
// pegawai aktif lain yang dalam 30 hari terakhir bekerja di domain yang sama. Paling banyak 5 tawaran baru per
// penerima per minggu dan 3 penerima baru per butir per putaran; pegawai yang pernah membuangnya tidak ditawari lagi.
func offerShares(db *gorm.DB, now time.Time) int {
	var nodes []models.KnowledgeNode
	db.Where("status = ? AND owner_employee_id IS NOT NULL AND helpful >= ? AND harmful = 0 AND domain <> '' AND created_at <= ?",
		models.KnowledgeNodeActive, shareMinHelpful, now.Add(-shareMinAge)).Order("helpful DESC, id").Find(&nodes)
	if len(nodes) == 0 {
		return 0
	}
	type activity struct {
		EmployeeID uint
		Domain     string
		N          int
	}
	var acts []activity
	db.Model(&models.KnowledgeUse{}).
		Select("knowledge_uses.employee_id AS employee_id, knowledge_nodes.domain AS domain, COUNT(*) AS n").
		Joins("JOIN knowledge_nodes ON knowledge_nodes.id = knowledge_uses.node_id").
		Joins("JOIN employees ON employees.id = knowledge_uses.employee_id").
		Where("knowledge_uses.created_at >= ? AND knowledge_uses.employee_id IS NOT NULL AND knowledge_nodes.domain <> '' AND employees.status = ?",
			now.Add(-shareActiveWindow), models.EmployeeActive).
		Group("knowledge_uses.employee_id, knowledge_nodes.domain").Scan(&acts)
	byDomain := map[string][]activity{}
	for _, a := range acts {
		byDomain[a.Domain] = append(byDomain[a.Domain], a)
	}
	for d := range byDomain {
		sort.Slice(byDomain[d], func(i, j int) bool { return byDomain[d][i].N > byDomain[d][j].N })
	}
	type weekly struct {
		EmployeeID uint
		N          int
	}
	var recent []weekly
	db.Model(&models.KnowledgeAdoption{}).Select("employee_id, COUNT(*) AS n").Where("created_at >= ?", now.Add(-7*24*time.Hour)).
		Group("employee_id").Scan(&recent)
	offeredThisWeek := map[uint]int{}
	for _, r := range recent {
		offeredThisWeek[r.EmployeeID] = r.N
	}
	offered := 0
	for _, n := range nodes {
		if !NodeInForce(n, now) || n.OwnerEmployeeID == nil || ProjectSpecific(n) {
			continue
		}
		var had []uint
		db.Model(&models.KnowledgeAdoption{}).Where("node_id = ?", n.ID).Pluck("employee_id", &had)
		skip := map[uint]bool{*n.OwnerEmployeeID: true}
		for _, e := range had {
			skip[e] = true
		}
		added := 0
		for _, a := range byDomain[n.Domain] {
			if added >= shareMaxPerNodeRound {
				break
			}
			if skip[a.EmployeeID] || offeredThisWeek[a.EmployeeID] >= shareMaxPerWeek {
				continue
			}
			row := models.KnowledgeAdoption{NodeID: n.ID, EmployeeID: a.EmployeeID, Status: models.KnowledgeAdoptionTrial,
				Reason: clip(fmt.Sprintf("ditawarkan: pengalaman %s di domain %s (✓%d)", n.OwnerName, n.Domain, n.Helpful), 250)}
			if db.Create(&row).Error == nil {
				offeredThisWeek[a.EmployeeID]++
				skip[a.EmployeeID] = true
				added++
				offered++
			}
		}
	}
	if offered > 0 {
		invalidate()
	}
	return offered
}

// decideTrials menutup masa coba: ✗ → dibuang; ✓ di pekerjaan penerima → dipertahankan; disodorkan 5 kali tanpa
// pernah dikutip, atau 21 hari tanpa bukti → dibuang (tidak relevan bagi penerima).
func decideTrials(db *gorm.DB, now time.Time) int {
	var trials []models.KnowledgeAdoption
	db.Where("status = ?", models.KnowledgeAdoptionTrial).Find(&trials)
	specific := map[uint]bool{}
	if len(trials) > 0 {
		ids := make([]uint, 0, len(trials))
		for _, t := range trials {
			ids = append(ids, t.NodeID)
		}
		var nodes []models.KnowledgeNode
		db.Select("id", "title", "content").Where("id IN ?", ids).Find(&nodes)
		for _, n := range nodes {
			specific[n.ID] = ProjectSpecific(n)
		}
	}
	decided := 0
	for _, t := range trials {
		status, reason := "", ""
		switch {
		case specific[t.NodeID]:
			status, reason = models.KnowledgeAdoptionDropped, "khusus satu aplikasi (menyebut berkas/baris/todo)"
		case t.Harmful > 0:
			status, reason = models.KnowledgeAdoptionDropped, "masa coba: dinilai keliru di pekerjaan penerima"
		case t.Helpful > 0:
			status, reason = models.KnowledgeAdoptionKept, fmt.Sprintf("masa coba: terbukti membantu (✓%d)", t.Helpful)
		case t.Uses >= trialMaxUses && t.Cited == 0:
			status, reason = models.KnowledgeAdoptionDropped, fmt.Sprintf("masa coba: %d kali disodorkan tanpa dikutip", t.Uses)
		case t.CreatedAt.Before(now.Add(-trialMaxAge)):
			status, reason = models.KnowledgeAdoptionDropped, "masa coba: 21 hari tanpa bukti"
		}
		if status == "" {
			continue
		}
		db.Model(&models.KnowledgeAdoption{}).Where("id = ?", t.ID).
			Updates(map[string]interface{}{"status": status, "reason": reason, "decided_at": &now})
		decided++
	}
	if decided > 0 {
		invalidate()
	}
	return decided
}

// Ambang promosi pengalaman menjadi patokan kantor dan penurunannya.
const (
	promoteMinHelpful       = 8                   // bukti kuat ✓ total
	promoteMinEmployees     = 3                   // dari sekian pegawai berbeda (pemilik + penerima)
	promoteMinProjects      = 2                   // di sekian proyek berbeda
	promoteMinUtility       = 0.85                // StrongUtility
	promoteCriticalHelpful  = 10                  // domain security & ui_ux
	promoteCriticalProjects = 3                   //
	promoteMaxDropRate      = 0.3                 // porsi penerima yang membuangnya
	promoteHarmfulWindow    = 30 * 24 * time.Hour // tanpa ✗ dalam jendela ini
	promoteReviewsPerRound  = 3                   // panggilan peninjau per putaran kurasi
	promoteRetryHelpful     = 3                   // ditolak peninjau → ditinjau lagi setelah ✓ bertambah sekian
	demoteMaxUtility        = 0.6                 // patokan hasil promosi di bawah ini perlu ditinjau
	demoteMinJudged         = 3                   // … bila sudah dinilai minimal sekian kali
)

// promotionRejectedPrefix menandai butir yang ditolak peninjau beserta jumlah ✓ saat itu.
const promotionRejectedPrefix = "Promosi ditolak peninjau pada ✓"

// PromotionReview adalah keputusan peninjau (Pro: model kelas Opus) atas calon patokan: setuju/tidak, lalu
// tulisan kanonis yang berlaku umum dan batas berlakunya.
type PromotionReview struct {
	Approve   bool
	Title     string
	Content   string
	AppliesTo string
	Reason    string
}

var promotionReviewer func(db *gorm.DB, n models.KnowledgeNode, context string) (PromotionReview, error)

// SetPromotionReviewer memasang peninjau calon patokan. Tanpa peninjau, calon yang lolos ambang dipromosikan apa adanya.
func SetPromotionReviewer(f func(db *gorm.DB, n models.KnowledgeNode, context string) (PromotionReview, error)) {
	promotionReviewer = f
}

// promotionSpread: pegawai & proyek berbeda yang memberi bukti ✓ kuat untuk butir ini, ✗ kuat terbaru, dan porsi
// penerima yang membuangnya.
type promotionSpread struct {
	Employees, Projects int
	RecentHarmful       int
	DropRate            float64
}

func spreadOf(db *gorm.DB, n models.KnowledgeNode, now time.Time) promotionSpread {
	var ctxs []string
	db.Model(&models.KnowledgeVerdict{}).Where("node_id = ? AND helpful = ?", n.ID, true).Pluck("context", &ctxs)
	var cited []string
	db.Model(&models.KnowledgeUse{}).Where("node_id = ? AND outcome = ? AND evidence IN ?", n.ID, models.KnowledgeOutcomeHelpful,
		[]string{models.KnowledgeEvidenceVerdict, models.KnowledgeEvidenceCited}).Pluck("context", &cited)
	todoIDs := map[uint64]bool{}
	for _, c := range append(ctxs, cited...) {
		if id, err := strconv.ParseUint(strings.TrimPrefix(c, "todo:"), 10, 64); err == nil && strings.HasPrefix(c, "todo:") {
			todoIDs[id] = true
		}
	}
	emps, projs := map[uint]bool{}, map[uint]bool{}
	if n.OwnerEmployeeID != nil {
		emps[*n.OwnerEmployeeID] = true
	}
	if len(todoIDs) > 0 {
		ids := make([]uint64, 0, len(todoIDs))
		for id := range todoIDs {
			ids = append(ids, id)
		}
		var todos []models.Todo
		db.Select("id", "project_id", "employee_id").Where("id IN ?", ids).Find(&todos)
		for _, t := range todos {
			if t.ProjectID != nil && *t.ProjectID > 0 {
				projs[*t.ProjectID] = true
			}
			if t.EmployeeID != nil && *t.EmployeeID > 0 {
				emps[*t.EmployeeID] = true
			}
		}
	}
	var adopters []models.KnowledgeAdoption
	db.Where("node_id = ?", n.ID).Find(&adopters)
	decided, dropped := 0, 0
	for _, a := range adopters {
		if a.Helpful > 0 {
			emps[a.EmployeeID] = true
		}
		if a.Status != models.KnowledgeAdoptionTrial {
			decided++
			if a.Status == models.KnowledgeAdoptionDropped {
				dropped++
			}
		}
	}
	var harmful int64
	db.Model(&models.KnowledgeVerdict{}).Where("node_id = ? AND helpful = ? AND updated_at >= ?", n.ID, false, now.Add(-promoteHarmfulWindow)).Count(&harmful)
	var harmfulUses int64
	db.Model(&models.KnowledgeUse{}).Where("node_id = ? AND outcome = ? AND evidence IN ? AND created_at >= ?", n.ID, models.KnowledgeOutcomeHarmful,
		[]string{models.KnowledgeEvidenceVerdict, models.KnowledgeEvidenceCited}, now.Add(-promoteHarmfulWindow)).Count(&harmfulUses)
	s := promotionSpread{Employees: len(emps), Projects: len(projs), RecentHarmful: int(harmful + harmfulUses)}
	if decided > 0 {
		s.DropRate = float64(dropped) / float64(decided)
	}
	return s
}

// criticalDomain: domain yang butuh bukti lebih banyak sebelum menjadi patokan.
func criticalDomain(d string) bool { return d == "security" || d == "ui_ux" }

// promotable: pengalaman pribadi yang lolos ambang bukti dan sebaran (sebelum peninjau).
func promotable(n models.KnowledgeNode, s promotionSpread) bool {
	minHelpful, minProjects := promoteMinHelpful, promoteMinProjects
	if criticalDomain(n.Domain) {
		minHelpful, minProjects = promoteCriticalHelpful, promoteCriticalProjects
	}
	return n.OwnerEmployeeID != nil && n.Helpful >= minHelpful && n.StrongUtility() >= promoteMinUtility &&
		s.Employees >= promoteMinEmployees && s.Projects >= minProjects && s.RecentHarmful == 0 && s.DropRate <= promoteMaxDropRate
}

// promotableCounts: syarat promosi yang hanya bergantung pada penghitung node (dipakai laporan hitung ulang bukti).
func promotableCounts(n models.KnowledgeNode) bool {
	minHelpful := promoteMinHelpful
	if criticalDomain(n.Domain) {
		minHelpful = promoteCriticalHelpful
	}
	return n.OwnerEmployeeID != nil && n.Helpful >= minHelpful && n.StrongUtility() >= promoteMinUtility
}

// rejectedAt: jumlah ✓ saat peninjau terakhir menolak (0 = belum pernah ditolak).
func rejectedAt(n models.KnowledgeNode) int {
	rest, ok := strings.CutPrefix(n.StatusReason, promotionRejectedPrefix)
	if !ok {
		return 0
	}
	num := rest
	if i := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		num = rest[:i]
	}
	v, _ := strconv.Atoi(num)
	return v
}

// promote: pengalaman yang terbukti di banyak pegawai & proyek ditinjau (Pro: model kelas Opus) lalu menjadi patokan
// bersama kantor; butir pribadi asalnya digantikan (supersede) oleh patokan itu.
func promote(db *gorm.DB) int {
	var rows []models.KnowledgeNode
	db.Where("status = ? AND owner_employee_id IS NOT NULL AND helpful >= ? AND superseded_by_id IS NULL", models.KnowledgeNodeActive, promoteMinHelpful).
		Order("helpful DESC, id").Find(&rows)
	n, reviews := 0, 0
	now := time.Now()
	for _, r := range rows {
		if !NodeInForce(r, now) {
			continue
		}
		var exists int64
		db.Model(&models.KnowledgeNode{}).Where("source_key = ?", "promoted:"+itoa(r.ID)).Count(&exists)
		if exists > 0 {
			continue
		}
		if at := rejectedAt(r); at > 0 && r.Helpful < at+promoteRetryHelpful {
			continue
		}
		if ProjectSpecific(r) {
			continue
		}
		s := spreadOf(db, r, now)
		if !promotable(r, s) {
			continue
		}
		note := Note{Key: "promoted:" + itoa(r.ID), Kind: r.Kind, Domain: r.Domain, Title: r.Title, Content: r.Content,
			Source: models.KnowledgeSrcPromoted, Tags: append([]string{"promosi"}, r.Tags...), OwnerName: r.OwnerName, Weight: r.Weight}
		if promotionReviewer != nil {
			if reviews >= promoteReviewsPerRound {
				break
			}
			reviews++
			ctx := fmt.Sprintf("✓%d dari %d pegawai di %d proyek; utilitas %.2f; porsi penerima membuang %.0f%%",
				r.Helpful, s.Employees, s.Projects, r.StrongUtility(), 100*s.DropRate)
			rev, err := promotionReviewer(db, r, ctx)
			if err != nil {
				continue // peninjau tidak tersedia: coba lagi di putaran berikutnya
			}
			if !rev.Approve {
				db.Model(&models.KnowledgeNode{}).Where("id = ?", r.ID).UpdateColumn("status_reason",
					clip(fmt.Sprintf("%s%d: %s", promotionRejectedPrefix, r.Helpful, rev.Reason), 1000))
				continue
			}
			if strings.TrimSpace(rev.Title) != "" && strings.TrimSpace(rev.Content) != "" {
				note.Title, note.Content = rev.Title, rev.Content
			}
			note.AppliesTo = rev.AppliesTo
		}
		row, created, err := upsertKey(db, note)
		if err != nil || !created {
			continue
		}
		if note.AppliesTo != "" {
			_ = SetValidity(db, row.ID, note.AppliesTo, nil)
		}
		_, _ = Supersede(db, r.ID, row.ID, "dinaikkan menjadi patokan kantor")
		n++
	}
	return n
}

// demote menandai patokan hasil promosi yang tidak lagi terbukti (utilitas < 0,6 setelah minimal 3 penilaian, atau
// ✗ kuat ≥ 3 dalam 30 hari) supaya ditinjau; tidak menghapus otomatis.
func demote(db *gorm.DB, now time.Time) int {
	var rows []models.KnowledgeNode
	db.Where("status = ? AND source = ? AND needs_review = ?", models.KnowledgeNodeActive, models.KnowledgeSrcPromoted, false).Find(&rows)
	n := 0
	for _, r := range rows {
		var recent int64
		db.Model(&models.KnowledgeVerdict{}).Where("node_id = ? AND helpful = ? AND updated_at >= ?", r.ID, false, now.Add(-promoteHarmfulWindow)).Count(&recent)
		low := r.Helpful+r.Harmful >= demoteMinJudged && r.StrongUtility() < demoteMaxUtility
		if !low && recent < 3 {
			continue
		}
		db.Model(&models.KnowledgeNode{}).Where("id = ?", r.ID).Updates(map[string]interface{}{"needs_review": true,
			"status_reason": clip(fmt.Sprintf("%s ✓%d ✗%d (✗ 30 hari: %d)", demoteReasonPrefix, r.Helpful, r.Harmful, recent), 1000)})
		n++
	}
	return n
}

// demoteReasonPrefix: alasan tanda tinjau patokan hasil promosi (tidak dilepas otomatis oleh prune).
const demoteReasonPrefix = "Patokan perlu ditinjau: tidak lagi terbukti"

// NetworkNode: satu butir di jaring pengalaman pegawai.
type NetworkNode struct {
	ID       uint   `json:"id"`
	Title    string `json:"title"`
	Domain   string `json:"domain"`
	Kind     string `json:"kind"`
	Relation string `json:"relation"` // own, adopted, trial, dropped, promoted
	Owner    string `json:"owner,omitempty"`
	Helpful  int    `json:"helpful"`
	Harmful  int    `json:"harmful"`
	Uses     int    `json:"uses"`
	Reason   string `json:"reason,omitempty"`
}

// NetworkEdge: tautan antarbutir di jaring pegawai (kemiripan isi, atau dinaikkan menjadi patokan).
type NetworkEdge struct {
	From     uint    `json:"from"`
	To       uint    `json:"to"`
	Kind     string  `json:"kind"` // similar, promoted
	Strength float64 `json:"strength,omitempty"`
}

// Network: jaring pengalaman satu pegawai (pengalaman sendiri, adopsi, dan patokan hasil promosi pengalamannya) dan
// ringkasan bidang tempat ia paling sering memakai patokan kantor (jaring pengetahuannya).
type Network struct {
	EmployeeID uint           `json:"employee_id"`
	Nodes      []NetworkNode  `json:"nodes"`
	Edges      []NetworkEdge  `json:"edges"`
	Domains    map[string]int `json:"domains"` // pemakaian patokan bersama per domain, 30 hari terakhir
}

// EmployeeNetwork menyusun jaring pengalaman & pengetahuan satu pegawai.
func EmployeeNetwork(db *gorm.DB, employeeID uint) Network {
	net := Network{EmployeeID: employeeID, Domains: map[string]int{}}
	if db == nil || employeeID == 0 {
		return net
	}
	var own []models.KnowledgeNode
	db.Where("owner_employee_id = ? AND status = ?", employeeID, models.KnowledgeNodeActive).Find(&own)
	ids := map[uint]bool{}
	add := func(n models.KnowledgeNode, rel, reason string, uses int) {
		if ids[n.ID] {
			return
		}
		ids[n.ID] = true
		net.Nodes = append(net.Nodes, NetworkNode{ID: n.ID, Title: n.Title, Domain: n.Domain, Kind: n.Kind, Relation: rel,
			Owner: n.OwnerName, Helpful: n.Helpful, Harmful: n.Harmful, Uses: uses, Reason: reason})
	}
	for _, n := range own {
		add(n, "own", "", n.Uses)
		if n.SupersededByID != nil {
			var p models.KnowledgeNode
			if db.First(&p, *n.SupersededByID).Error == nil && p.Source == models.KnowledgeSrcPromoted {
				add(p, "promoted", "", p.Uses)
				net.Edges = append(net.Edges, NetworkEdge{From: n.ID, To: p.ID, Kind: "promoted"})
			}
		}
	}
	var ads []models.KnowledgeAdoption
	db.Where("employee_id = ?", employeeID).Find(&ads)
	for _, a := range ads {
		var n models.KnowledgeNode
		if db.First(&n, a.NodeID).Error != nil {
			continue
		}
		rel := map[string]string{models.KnowledgeAdoptionKept: "adopted", models.KnowledgeAdoptionTrial: "trial",
			models.KnowledgeAdoptionDropped: "dropped"}[a.Status]
		add(n, rel, a.Reason, a.Uses)
	}
	if len(ids) > 0 {
		list := make([]uint, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		var links []models.KnowledgeLink
		db.Where("from_id IN ? AND to_id IN ?", list, list).Find(&links)
		for _, l := range links {
			net.Edges = append(net.Edges, NetworkEdge{From: l.FromID, To: l.ToID, Kind: "similar", Strength: l.Strength})
		}
	}
	type dom struct {
		Domain string
		N      int
	}
	var ds []dom
	db.Model(&models.KnowledgeUse{}).Select("knowledge_nodes.domain AS domain, COUNT(*) AS n").
		Joins("JOIN knowledge_nodes ON knowledge_nodes.id = knowledge_uses.node_id").
		Where("knowledge_uses.employee_id = ? AND knowledge_uses.created_at >= ? AND knowledge_nodes.owner_employee_id IS NULL AND knowledge_nodes.domain <> ''",
			employeeID, time.Now().Add(-shareActiveWindow)).
		Group("knowledge_nodes.domain").Scan(&ds)
	for _, d := range ds {
		net.Domains[d.Domain] = d.N
	}
	return net
}
