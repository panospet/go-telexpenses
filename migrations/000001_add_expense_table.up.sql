create table expense (
    id serial primary key,
    user_str varchar(500) not null,
    category varchar(200) not null,
    amount decimal not null,
    comment text not null,
    created_at timestamp not null default now()
);

create index expense_category_idx on expense (category);
create index expense_user_id_idx on expense (user_str);
create index expense_created_at_idx on expense (created_at);
create index expense_amount_idx on expense (amount);