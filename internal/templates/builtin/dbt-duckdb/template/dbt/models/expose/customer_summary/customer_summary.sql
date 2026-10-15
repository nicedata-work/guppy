-- Placeholder expose-layer mart over the model layer.
select
  c.customer_id,
  c.customer_name,
  c.segment,
  count(*) as order_count
from {{ ref('dim_customer') }} as c
group by 1, 2, 3
