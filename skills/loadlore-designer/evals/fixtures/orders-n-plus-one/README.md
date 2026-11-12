# orders

The orders service: `GET /orders?customer=<id>` returns a customer's 20 most recent orders with their items. Two replicas run behind the load balancer, against one Postgres.
