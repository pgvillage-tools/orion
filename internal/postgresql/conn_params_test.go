package postgresql

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConnParams", func() {
	When("Using Set Get and Del", func() {
		It("should work as expected", func() {
			const (
				key   = "mykey1"
				value = "myvalue1"
			)
			cp := ConnParams{}
			Ω(cp.Get(key)).To(BeEmpty())
			Ω(cp.Isset(key)).To(BeFalse())
			cp.Set(key, value)
			Ω(cp.Get(key)).To(Equal(value))
			Ω(cp.Isset(key)).To(BeTrue())
			cp.Del(key)
			Ω(cp.Get(key)).To(BeEmpty())
			Ω(cp.Isset(key)).To(BeFalse())
		})
	})
	When("Comparing", func() {
		It("should work as expected", func() {
			cp1 := ConnParams{
				ConnParamKeyHost: "::1",
				ConnParamKeyPort: "5432",
			}
			cp2 := cp1.Copy()
			Ω(cp1.Equals(cp2)).To(BeTrue())
			cp2[ConnParamKeyHost] = "127.0.0.1"
			Ω(cp1.Equals(cp2)).To(BeFalse())
			cp2[ConnParamKeyHost] = cp1[ConnParamKeyHost]
			Ω(cp1.Equals(cp2)).To(BeTrue())
		})
	})
	When("Parsing", func() {
		It("should work as expected", func() {
			cn, err := ParseConnString(`host=myhost port=5433 f=v`)
			Ω(err).NotTo(HaveOccurred())
			Ω(cn).To(HaveKeyWithValue(ConnParamKeyHost, "myhost"))
			Ω(cn).To(HaveKeyWithValue(ConnParamKey("f"), "v"))
		})
	})
	When("Parsing url", func() {
		It("should work as expected", func() {
			for _, url := range []string{
				`postgres://myself:password@myhost:5434/mydb1`,
				`postgresql://myself:password@myhost:5434/mydb1`,
			} {
				cn, err := URLToConnParams(url)
				Ω(err).NotTo(HaveOccurred())
				Ω(cn).To(HaveKeyWithValue(ConnParamKeyHost, "myhost"))
				Ω(cn).To(HaveKeyWithValue(ConnParamKeyPort, "5434"))
				Ω(cn).To(HaveKeyWithValue(ConnParamKeyUser, "myself"))
			}
		})
	})
	When("Getting as connstring", func() {
		It("should work as expected", func() {
			cn, err := URLToConnParams(`postgresql://myself:password@myhost:5434/mydb1`)
			Ω(err).NotTo(HaveOccurred())
			Ω(cn.ConnString()).To(Equal("dbname=mydb1 host=myhost password=password port=5434 user=myself"))
		})
	})
	When("Getting a clone with other settings", func() {
		It("should work as expected", func() {
			cp1 := ConnParams{
				ConnParamKeyHost: "::1",
				ConnParamKeyPort: "5432",
				ConnParamKeyUser: "myself",
			}
			cp2 := cp1.WithUser("someoneelse").
				WithAppName("myapp").
				WithPort(5433).
				WithSSLMode("verify-full")
			Ω(cp1).To(HaveKeyWithValue(ConnParamKeyUser, "myself"))
			Ω(cp2).To(HaveKeyWithValue(ConnParamKeyUser, "someoneelse"))
			Ω(cp2).To(HaveKeyWithValue(ConnParamKeyHost, cp1[ConnParamKeyHost]))
			Ω(cp2).To(HaveKeyWithValue(ConnParamKeyPort, "5433"))
		})
		DescribeTable("each With* function only sets its own key on a clone",
			func(with func(ConnParams) ConnParams, key ConnParamKey, value string) {
				cp1 := ConnParams{ConnParamKeyHost: "::1", ConnParamKeyPort: "5432"}
				expected := cp1.Copy()
				expected[key] = value
				Ω(with(cp1)).To(Equal(expected))
				Ω(cp1).To(Equal(ConnParams{ConnParamKeyHost: "::1", ConnParamKeyPort: "5432"}))
			},
			Entry("WithUser", func(cp ConnParams) ConnParams { return cp.WithUser("me") }, ConnParamKeyUser, "me"),
			Entry("WithHost", func(cp ConnParams) ConnParams { return cp.WithHost("db1") }, ConnParamKeyHost, "db1"),
			Entry("WithDbName", func(cp ConnParams) ConnParams { return cp.WithDbName("mydb") }, ConnParamKeyDbName, "mydb"),
			Entry("WithPort", func(cp ConnParams) ConnParams { return cp.WithPort(6432) }, ConnParamKeyPort, "6432"),
			Entry("WithSPort", func(cp ConnParams) ConnParams { return cp.WithSPort("7432") }, ConnParamKeyPort, "7432"),
			Entry("WithAppName", func(cp ConnParams) ConnParams { return cp.WithAppName("app") }, ConnParamKeyAppName, "app"),
			Entry("WithSSLMode", func(cp ConnParams) ConnParams { return cp.WithSSLMode("require") },
				ConnParamKeySSLMode, "require"),
			Entry("WithPassword", func(cp ConnParams) ConnParams { return cp.WithPassword("secret") },
				ConnParamKeyPassword, "secret"),
		)
	})
	When("Getting as connstring with special characters and empty values", func() {
		It("should escape the special characters and skip the empty values", func() {
			cp := ConnParams{
				ConnParamKeyPassword: `a b'c\d`,
				ConnParamKeyUser:     "",
			}
			Ω(cp.ConnString()).To(Equal(`password=a\ b\'c\\d`))
		})
	})
})
