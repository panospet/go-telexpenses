create table expense (
    id serial primary key,
    category varchar(200) not null,
    amount float not null,
    comment text not null,
    created_at timestamp not null default now()
);

create index expense_category_idx on expense (category);
create index expense_created_at_idx on expense (created_at);
create index expense_amount_idx on expense (amount);