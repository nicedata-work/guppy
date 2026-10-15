-- Placeholder semantic layer: a governed view over the expose mart.
-- (DuckDB's semantic_views extension can upgrade this to metrics/dimensions.)
select
  customer_id,
  customer_name,
  segment,
  order_count
from {{ ref('customer_summary') }}
