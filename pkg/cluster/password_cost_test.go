package cluster

import "github.com/TykTechnologies/midsommar/v2/models"

// Hash test passwords at bcrypt.MinCost instead of DefaultCost (~0.8 s per
// hash under -race). See models.UseMinPasswordCostForTests.
func init() { models.UseMinPasswordCostForTests() }
