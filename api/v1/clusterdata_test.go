package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Clusterdata", func() {
	When("Copying a Data", func() {
		var (
			orgData, newData *Data
		)
		BeforeEach(func() {
			orgData = randomData()
			newData = orgData.DeepCopy()
		})
		It("should be a copy", func() {
			newData.FormatVersion = randomUInt64()
			Ω(newData).NotTo(Equal(orgData))
		})
		It("should copy all fields", func() {
			Ω(newData).To(Equal(orgData))
		})
		It("should return nil when Data is nil", func() {
			var d *Data
			Ω(d.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when Data is empty struct", func() {
			var d Data
			Ω(d.DeepCopy()).To(Equal(&Data{}))
		})
	})
	When("Finding the db of a keeper", func() {
		keeper1 := &Keeper{UID: "keeper1"}
		It("returns the db whose spec has the keeper", func() {
			d := &Data{DBs: DBs{
				"db1": {UID: "db1", Spec: &DBSpec{KeeperUID: "keeper2"}},
				"db2": {UID: "db2", Spec: &DBSpec{KeeperUID: "keeper1"}},
			}}
			Ω(d.FindDB(keeper1)).To(HaveField("UID", "db2"))
		})
		It("returns nil for a keeper without a db", func() {
			d := &Data{DBs: DBs{"db1": {UID: "db1", Spec: &DBSpec{KeeperUID: "keeper2"}}}}
			Ω(d.FindDB(keeper1)).To(BeNil())
			Ω((&Data{}).FindDB(keeper1)).To(BeNil())
		})
		It("skips a db without a spec, or a nil db, instead of panicking", func() {
			d := &Data{DBs: DBs{
				"db0": nil,
				"db1": {UID: "db1"},
				"db2": {UID: "db2", Spec: &DBSpec{KeeperUID: "keeper1"}},
			}}
			Ω(d.FindDB(keeper1)).To(HaveField("UID", "db2"))
			delete(d.DBs, "db2")
			Ω(d.FindDB(keeper1)).To(BeNil())
		})
	})
	When("Defining a new cluster data object", func() {
		It("should return a well defined object", func() {
			c := randomCluster()
			d := NewClusterData(c)
			Ω(d.FormatVersion).To(Equal(CurrentCDFormatVersion))
			Ω(d.Cluster).To(Equal(c))
			Ω(d.Keepers).NotTo(BeNil())
			Ω(d.DBs).NotTo(BeNil())
			Ω(d.Proxy).NotTo(BeNil())
		})
	})
})
