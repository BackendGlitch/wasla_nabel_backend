package pricing

// ServiceFeePerSeatTND is the default per-seat station fee in TND (200 millimes) when
// routes.service_fee is not set. Destinations can override in DB (e.g. 0.05, 0.1, 0.2 for 50/100/200 millimes).
// Keep in sync with booking charges, tickets, staff UIs, and statistics SQL COALESCE(..., 0.2).
const ServiceFeePerSeatTND = 0.2
