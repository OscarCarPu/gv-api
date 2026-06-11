// Package finance manages accounts, categories, transactions and money stats.
//
// Endpoints:
//
//	GET    /finance/accounts                - list accounts
//	GET    /finance/accounts/{id}           - get account
//	POST   /finance/accounts                - create account
//	PUT    /finance/accounts/{id}           - update account
//	DELETE /finance/accounts/{id}           - delete account (fails if it has transactions)
//	GET    /finance/overview                - totals + recent transactions
//	GET    /finance/categories              - list categories
//	GET    /finance/categories/{id}         - get category
//	POST   /finance/categories              - create category
//	PUT    /finance/categories/{id}         - update category
//	DELETE /finance/categories/{id}         - delete category (fails if referenced)
//	GET    /finance/transactions            - list transactions (date/type/account/category filters)
//	GET    /finance/transactions/{id}       - get transaction
//	POST   /finance/transactions            - create transaction
//	PUT    /finance/transactions/{id}       - update transaction
//	DELETE /finance/transactions/{id}       - delete transaction
//	GET    /finance/stats/networth          - net worth series
//	GET    /finance/stats/by-category       - spend/income by category
//	GET    /finance/stats/monthly           - monthly totals
//	GET    /finance/stats/estimation        - net worth projection
package finance
