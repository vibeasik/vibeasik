package models

import "testing"

func TestTodoModelTier(t *testing.T) {
	cases := []struct {
		name string
		todo Todo
		want string
	}{
		{"fitur berat tetap model utama", Todo{Title: "Rancang ulang alur pembayaran", Category: CategoryArchitecture, TodoComplexity: TodoComplexity{ComplexityLevel: ComplexityComplex, ComplexityScore: 4}}, ModelTierMain},
		{"security kritis", Todo{Title: "Tutup celah SQL injection", Category: CategorySecurity}, ModelTierCritical},
		{"prioritas critical", Todo{Title: "Perbaiki login gagal", Category: CategoryFeature, Priority: TodoPriorityCritical}, ModelTierCritical},
		{"ekstraksi data", Todo{Title: "Ekstrak daftar harga dari PDF ke JSON", Category: CategoryFeature, TodoComplexity: TodoComplexity{ComplexityLevel: ComplexityNormal, ComplexityScore: 2}}, ModelTierExtract},
		{"ekstraksi tapi berat", Todo{Title: "Migrasi data pelanggan lintas skema", Category: CategoryBackend, TodoComplexity: TodoComplexity{ComplexityLevel: ComplexityComplex, ComplexityScore: 4}}, ModelTierMain},
	}
	for _, c := range cases {
		if got := TodoModelTier(&c.todo); got != c.want {
			t.Errorf("%s: %s, mau %s", c.name, got, c.want)
		}
	}
}
