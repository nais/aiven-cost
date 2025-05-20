# AIVEN COST
pseudo:

```
Hent alle billing groups
Hent alle invoices med id per group
select id & status i BQ
if bq.status != "paid" {
  hent invoiceLines
  bygg struct
  lagre i bq
}
```
