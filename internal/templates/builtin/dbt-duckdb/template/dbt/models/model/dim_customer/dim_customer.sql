{{ config(materialized='table') }}

-- Placeholder dimension: replace with a real source (seed, source table,
-- or upstream model).
with customers as (
  select
    t.customer_id,
    t.customer_name,
    t.segment
  from (
    values
    ('C001', 'acme', 'enterprise'),
    ('C002', 'globex', 'smb'),
    ('C003', 'initech', 'enterprise'),
  ) as t (customer_id, customer_name, segment)
)

select
  customer_id,
  customer_name,
  segment
from customers
