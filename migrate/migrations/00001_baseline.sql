-- +goose Up
-- Generated 2026-07-18 via pg_dump --schema-only against the live Railway prod DB.
-- This is the schema as it stood after years of migrate.Migrate()'s AutoMigrate +
-- ad-hoc ALTERs — the source of truth from here on is this migrations/ dir.
--
-- PostgreSQL database dump
--


-- Dumped from database version 18.3 (Debian 18.3-1.pgdg13+1)
-- Dumped by pg_dump version 18.4 (Debian 18.4-1.pgdg13+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;


--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: abandoned_leads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.abandoned_leads (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id text NOT NULL,
    product_id text NOT NULL,
    product_title text,
    price numeric,
    combination_str text,
    full_name text NOT NULL,
    phone_number text NOT NULL,
    f_bclid text,
    f_bp text,
    f_bc text,
    conversion_source text,
    state text,
    city text,
    shipping_method text,
    quantity bigint
);


--
-- Name: audit_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_logs (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    actor_id uuid NOT NULL,
    actor_email text NOT NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id uuid,
    metadata text,
    ip_address text
);


--
-- Name: available_delivery_companies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.available_delivery_companies (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text NOT NULL,
    url text NOT NULL,
    is_active boolean DEFAULT true NOT NULL
);


--
-- Name: available_delivery_company_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.available_delivery_company_images (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    available_delivery_company_id uuid CONSTRAINT available_delivery_company__available_delivery_company_not_null NOT NULL,
    url text NOT NULL
);


--
-- Name: clients; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.clients (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    full_name text,
    phone_number text NOT NULL,
    phone_number2 text,
    state text,
    state_code text,
    city text,
    stopdesk_point text
);


--
-- Name: coupon_landing_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.coupon_landing_pages (
    coupon_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    landing_page_id uuid DEFAULT public.uuid_generate_v4() NOT NULL
);


--
-- Name: coupon_products; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.coupon_products (
    coupon_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    product_id uuid DEFAULT public.uuid_generate_v4() NOT NULL
);


--
-- Name: coupons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.coupons (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id text NOT NULL,
    code text NOT NULL,
    percent bigint NOT NULL,
    active boolean DEFAULT true
);


--
-- Name: delivery_companies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_companies (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    available_delivery_company_id uuid NOT NULL,
    token text,
    merchant_id text
);


--
-- Name: delivery_rates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.delivery_rates (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    wilaya_id bigint NOT NULL,
    wilaya_name text NOT NULL,
    is_active boolean DEFAULT true,
    has_doorstep boolean DEFAULT true,
    doorstep_rate numeric DEFAULT 0,
    has_stopdesk boolean DEFAULT false,
    stopdesk_rate numeric DEFAULT 0
);


--
-- Name: feature_flags; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feature_flags (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    key text NOT NULL,
    label text NOT NULL,
    description text,
    is_enabled boolean DEFAULT false
);


--
-- Name: flagged_clients; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.flagged_clients (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id text NOT NULL,
    platform text NOT NULL,
    client_id text NOT NULL
);


--
-- Name: global_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.global_settings (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    key text NOT NULL,
    value text,
    value_type text DEFAULT 'string'::text,
    description text
);


--
-- Name: landing_page_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.landing_page_images (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    landing_page_id uuid NOT NULL,
    url text NOT NULL,
    order_index bigint DEFAULT 0 NOT NULL
);


--
-- Name: landing_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.landing_pages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    product_id uuid NOT NULL,
    title text NOT NULL,
    active boolean DEFAULT true
);


--
-- Name: offer_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offer_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id text NOT NULL,
    offer_id uuid NOT NULL,
    order_id uuid,
    event text NOT NULL,
    variant_id uuid,
    wilaya bigint,
    amount numeric DEFAULT 0 NOT NULL
);


--
-- Name: offer_page_overrides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offer_page_overrides (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id text NOT NULL,
    offer_id uuid NOT NULL,
    landing_page_id uuid NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    headline text,
    subheadline text,
    button_text text,
    offer_product_id uuid,
    discount_type text,
    discount_value numeric,
    placement text
);


--
-- Name: offers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id text NOT NULL,
    internal_name text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    action text NOT NULL,
    trigger_product_id uuid NOT NULL,
    landing_page_id uuid,
    offer_product_id uuid NOT NULL,
    offer_variant_ids text,
    quantity_rule bigint DEFAULT 1 NOT NULL,
    discount_type text DEFAULT 'percent'::text NOT NULL,
    discount_value numeric DEFAULT 0 NOT NULL,
    headline text NOT NULL,
    subheadline text,
    button_text text DEFAULT 'Add to my order'::text NOT NULL,
    media_url text,
    placement text NOT NULL,
    priority bigint DEFAULT 100 NOT NULL,
    conditions text,
    inventory_behavior text DEFAULT 'skip_when_oos'::text NOT NULL,
    analytics_tag text,
    start_at timestamp with time zone,
    end_at timestamp with time zone
);


--
-- Name: order_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_items (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    order_id uuid NOT NULL,
    product_id uuid NOT NULL,
    product_variant_combination_id uuid NOT NULL,
    quantity bigint DEFAULT 1 NOT NULL,
    price numeric NOT NULL
);


--
-- Name: orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orders (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    client_id uuid NOT NULL,
    shipping_method text,
    shipping_price numeric,
    total_price numeric,
    status text DEFAULT 'En attente'::text,
    note text,
    tracking_number text,
    ouvrable boolean DEFAULT false,
    fragile boolean DEFAULT false,
    essayable boolean DEFAULT false,
    f_bclid text,
    f_bc text,
    f_bp text,
    conversion_source text,
    is_shipped boolean DEFAULT false,
    shipped_at timestamp with time zone,
    shipped_via_id uuid,
    reported_date timestamp with time zone,
    coupon_id uuid,
    discount_amount numeric DEFAULT 0,
    is_hidden boolean DEFAULT false,
    t_tclid text,
    t_tp text,
    client_ip text,
    client_user_agent text,
    meta_purchase_sent_at timestamp with time zone
);


--
-- Name: permission_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permission_actions (
    name text NOT NULL,
    resource text DEFAULT ''::text NOT NULL,
    label text DEFAULT ''::text NOT NULL
);


--
-- Name: pixels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pixels (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    platform text NOT NULL,
    title text NOT NULL,
    pixel_id text NOT NULL,
    has_access_token boolean DEFAULT false,
    access_token text,
    is_active boolean DEFAULT true
);


--
-- Name: plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.plans (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text NOT NULL,
    price numeric DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true,
    max_shops bigint DEFAULT 1 NOT NULL,
    max_products bigint DEFAULT '-1'::integer NOT NULL,
    max_orders bigint DEFAULT '-1'::integer NOT NULL,
    max_landing_pages bigint DEFAULT '-1'::integer NOT NULL,
    max_users bigint DEFAULT '-1'::integer NOT NULL,
    max_facebook_pixels bigint DEFAULT 1 NOT NULL,
    max_tik_tok_pixels bigint DEFAULT 1 NOT NULL,
    has_confirmation_orders boolean DEFAULT true,
    has_abandoned_orders boolean DEFAULT false,
    has_order_tracking boolean DEFAULT false,
    has_client_tracking boolean DEFAULT false
);


--
-- Name: product_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_images (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    product_id uuid NOT NULL,
    url text NOT NULL,
    order_index bigint DEFAULT 0 NOT NULL
);


--
-- Name: product_variant_combinations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_variant_combinations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    product_id uuid NOT NULL,
    sku text NOT NULL,
    price numeric NOT NULL,
    quantity bigint DEFAULT 0,
    option1_id uuid,
    option2_id uuid,
    option3_id uuid,
    combination_string text
);


--
-- Name: products; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.products (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    price numeric NOT NULL,
    old_price numeric DEFAULT 0,
    active boolean DEFAULT true
);


--
-- Name: role_action_defaults; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_action_defaults (
    role text NOT NULL,
    action text NOT NULL,
    allow boolean
);


--
-- Name: shop_logo_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shop_logo_images (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    url text NOT NULL
);


--
-- Name: shop_member_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shop_member_permissions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_member_id uuid NOT NULL,
    action text NOT NULL,
    allow boolean
);


--
-- Name: shop_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shop_members (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'moderator'::text NOT NULL
);


--
-- Name: shop_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shop_roles (
    name text NOT NULL
);


--
-- Name: shop_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shop_subscriptions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    started_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    expiry_reminder_sent_at timestamp with time zone
);


--
-- Name: shop_visits; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shop_visits (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    shop_id uuid NOT NULL,
    day date NOT NULL,
    visitor_id text NOT NULL,
    created_at timestamp with time zone
);


--
-- Name: shops; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shops (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    owner_id uuid NOT NULL,
    is_active boolean DEFAULT true,
    is_verified boolean DEFAULT false,
    phone text,
    email text,
    address text
);


--
-- Name: support_ticket_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.support_ticket_messages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    ticket_id uuid NOT NULL,
    author_user_id uuid NOT NULL,
    body text NOT NULL,
    is_internal_note boolean DEFAULT false
);


--
-- Name: support_tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.support_tickets (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid,
    requester_user_id uuid NOT NULL,
    subject text NOT NULL,
    status text DEFAULT 'open'::text,
    priority text DEFAULT 'normal'::text,
    assigned_to_user_id uuid
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    first_name text NOT NULL,
    last_name text NOT NULL,
    phone_number text NOT NULL,
    email text NOT NULL,
    password text,
    role text DEFAULT 'moderator'::text,
    is_verified boolean DEFAULT false,
    email_otp text,
    email_otp_expires_at timestamp with time zone,
    platform_role text DEFAULT ''::text,
    is_suspended boolean DEFAULT false
);


--
-- Name: variant_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.variant_items (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    variant_id uuid NOT NULL,
    value text NOT NULL
);


--
-- Name: variants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.variants (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    product_id uuid NOT NULL,
    title text NOT NULL
);


--
-- Name: abandoned_leads abandoned_leads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.abandoned_leads
    ADD CONSTRAINT abandoned_leads_pkey PRIMARY KEY (id);


--
-- Name: audit_logs audit_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_logs
    ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id);


--
-- Name: available_delivery_companies available_delivery_companies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.available_delivery_companies
    ADD CONSTRAINT available_delivery_companies_pkey PRIMARY KEY (id);


--
-- Name: available_delivery_company_images available_delivery_company_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.available_delivery_company_images
    ADD CONSTRAINT available_delivery_company_images_pkey PRIMARY KEY (id);


--
-- Name: clients clients_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clients
    ADD CONSTRAINT clients_pkey PRIMARY KEY (id);


--
-- Name: coupon_landing_pages coupon_landing_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_landing_pages
    ADD CONSTRAINT coupon_landing_pages_pkey PRIMARY KEY (coupon_id, landing_page_id);


--
-- Name: coupon_products coupon_products_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_products
    ADD CONSTRAINT coupon_products_pkey PRIMARY KEY (coupon_id, product_id);


--
-- Name: coupons coupons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupons
    ADD CONSTRAINT coupons_pkey PRIMARY KEY (id);


--
-- Name: delivery_companies delivery_companies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_companies
    ADD CONSTRAINT delivery_companies_pkey PRIMARY KEY (id);


--
-- Name: delivery_rates delivery_rates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_rates
    ADD CONSTRAINT delivery_rates_pkey PRIMARY KEY (id);


--
-- Name: feature_flags feature_flags_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feature_flags
    ADD CONSTRAINT feature_flags_pkey PRIMARY KEY (id);


--
-- Name: flagged_clients flagged_clients_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.flagged_clients
    ADD CONSTRAINT flagged_clients_pkey PRIMARY KEY (id);


--
-- Name: global_settings global_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.global_settings
    ADD CONSTRAINT global_settings_pkey PRIMARY KEY (id);


--
-- Name: landing_page_images landing_page_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.landing_page_images
    ADD CONSTRAINT landing_page_images_pkey PRIMARY KEY (id);


--
-- Name: landing_pages landing_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.landing_pages
    ADD CONSTRAINT landing_pages_pkey PRIMARY KEY (id);


--
-- Name: offer_events offer_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offer_events
    ADD CONSTRAINT offer_events_pkey PRIMARY KEY (id);


--
-- Name: offer_page_overrides offer_page_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offer_page_overrides
    ADD CONSTRAINT offer_page_overrides_pkey PRIMARY KEY (id);


--
-- Name: offers offers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_pkey PRIMARY KEY (id);


--
-- Name: order_items order_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_pkey PRIMARY KEY (id);


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);


--
-- Name: permission_actions permission_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permission_actions
    ADD CONSTRAINT permission_actions_pkey PRIMARY KEY (name);


--
-- Name: pixels pixels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pixels
    ADD CONSTRAINT pixels_pkey PRIMARY KEY (id);


--
-- Name: plans plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);


--
-- Name: product_images product_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_images
    ADD CONSTRAINT product_images_pkey PRIMARY KEY (id);


--
-- Name: product_variant_combinations product_variant_combinations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_variant_combinations
    ADD CONSTRAINT product_variant_combinations_pkey PRIMARY KEY (id);


--
-- Name: products products_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT products_pkey PRIMARY KEY (id);


--
-- Name: role_action_defaults role_action_defaults_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_action_defaults
    ADD CONSTRAINT role_action_defaults_pkey PRIMARY KEY (role, action);


--
-- Name: shop_logo_images shop_logo_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_logo_images
    ADD CONSTRAINT shop_logo_images_pkey PRIMARY KEY (id);


--
-- Name: shop_member_permissions shop_member_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_member_permissions
    ADD CONSTRAINT shop_member_permissions_pkey PRIMARY KEY (id);


--
-- Name: shop_members shop_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_members
    ADD CONSTRAINT shop_members_pkey PRIMARY KEY (id);


--
-- Name: shop_roles shop_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_roles
    ADD CONSTRAINT shop_roles_pkey PRIMARY KEY (name);


--
-- Name: shop_subscriptions shop_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_subscriptions
    ADD CONSTRAINT shop_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: shop_visits shop_visits_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_visits
    ADD CONSTRAINT shop_visits_pkey PRIMARY KEY (id);


--
-- Name: shops shops_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shops
    ADD CONSTRAINT shops_pkey PRIMARY KEY (id);


--
-- Name: support_ticket_messages support_ticket_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_ticket_messages
    ADD CONSTRAINT support_ticket_messages_pkey PRIMARY KEY (id);


--
-- Name: support_tickets support_tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_tickets
    ADD CONSTRAINT support_tickets_pkey PRIMARY KEY (id);


--
-- Name: users uni_users_email; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT uni_users_email UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: variant_items variant_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variant_items
    ADD CONSTRAINT variant_items_pkey PRIMARY KEY (id);


--
-- Name: variants variants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variants
    ADD CONSTRAINT variants_pkey PRIMARY KEY (id);


--
-- Name: idx_abandoned_leads_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_abandoned_leads_deleted_at ON public.abandoned_leads USING btree (deleted_at);


--
-- Name: idx_abandoned_leads_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_abandoned_leads_shop_id ON public.abandoned_leads USING btree (shop_id);


--
-- Name: idx_audit_logs_action; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_action ON public.audit_logs USING btree (action);


--
-- Name: idx_audit_logs_actor_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_actor_id ON public.audit_logs USING btree (actor_id);


--
-- Name: idx_audit_logs_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_deleted_at ON public.audit_logs USING btree (deleted_at);


--
-- Name: idx_audit_logs_target_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_target_id ON public.audit_logs USING btree (target_id);


--
-- Name: idx_audit_logs_target_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_logs_target_type ON public.audit_logs USING btree (target_type);


--
-- Name: idx_available_delivery_companies_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_available_delivery_companies_deleted_at ON public.available_delivery_companies USING btree (deleted_at);


--
-- Name: idx_available_delivery_company_images_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_available_delivery_company_images_deleted_at ON public.available_delivery_company_images USING btree (deleted_at);


--
-- Name: idx_clients_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_clients_deleted_at ON public.clients USING btree (deleted_at);


--
-- Name: idx_coupons_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_coupons_code ON public.coupons USING btree (code);


--
-- Name: idx_coupons_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_coupons_deleted_at ON public.coupons USING btree (deleted_at);


--
-- Name: idx_coupons_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_coupons_shop_id ON public.coupons USING btree (shop_id);


--
-- Name: idx_delivery_companies_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_companies_deleted_at ON public.delivery_companies USING btree (deleted_at);


--
-- Name: idx_delivery_companies_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_companies_shop_id ON public.delivery_companies USING btree (shop_id);


--
-- Name: idx_delivery_rates_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_delivery_rates_deleted_at ON public.delivery_rates USING btree (deleted_at);


--
-- Name: idx_feature_flags_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feature_flags_deleted_at ON public.feature_flags USING btree (deleted_at);


--
-- Name: idx_feature_flags_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_feature_flags_key ON public.feature_flags USING btree (key);


--
-- Name: idx_flagged_client_shop_platform_cid; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_flagged_client_shop_platform_cid ON public.flagged_clients USING btree (shop_id, platform, client_id);


--
-- Name: idx_flagged_clients_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_flagged_clients_deleted_at ON public.flagged_clients USING btree (deleted_at);


--
-- Name: idx_global_settings_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_global_settings_deleted_at ON public.global_settings USING btree (deleted_at);


--
-- Name: idx_global_settings_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_global_settings_key ON public.global_settings USING btree (key);


--
-- Name: idx_landing_page_images_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_landing_page_images_deleted_at ON public.landing_page_images USING btree (deleted_at);


--
-- Name: idx_landing_pages_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_landing_pages_deleted_at ON public.landing_pages USING btree (deleted_at);


--
-- Name: idx_member_action; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_member_action ON public.shop_member_permissions USING btree (shop_member_id, action);


--
-- Name: idx_offer_events_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offer_events_deleted_at ON public.offer_events USING btree (deleted_at);


--
-- Name: idx_offer_events_offer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offer_events_offer_id ON public.offer_events USING btree (offer_id);


--
-- Name: idx_offer_events_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offer_events_order_id ON public.offer_events USING btree (order_id);


--
-- Name: idx_offer_events_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offer_events_shop_id ON public.offer_events USING btree (shop_id);


--
-- Name: idx_offer_page_overrides_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offer_page_overrides_deleted_at ON public.offer_page_overrides USING btree (deleted_at);


--
-- Name: idx_offer_page_overrides_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offer_page_overrides_shop_id ON public.offer_page_overrides USING btree (shop_id);


--
-- Name: idx_offers_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_deleted_at ON public.offers USING btree (deleted_at);


--
-- Name: idx_offers_landing_page_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_landing_page_id ON public.offers USING btree (landing_page_id);


--
-- Name: idx_offers_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_shop_id ON public.offers USING btree (shop_id);


--
-- Name: idx_offers_trigger_product_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_offers_trigger_product_id ON public.offers USING btree (trigger_product_id);


--
-- Name: idx_one_super_admin; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_one_super_admin ON public.users USING btree (platform_role) WHERE ((platform_role = 'super_admin'::text) AND (deleted_at IS NULL));


--
-- Name: idx_order_items_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_items_deleted_at ON public.order_items USING btree (deleted_at);


--
-- Name: idx_order_items_order_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_items_order_id ON public.order_items USING btree (order_id);


--
-- Name: idx_order_items_product_variant_combination_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_order_items_product_variant_combination_id ON public.order_items USING btree (product_variant_combination_id);


--
-- Name: idx_orders_client_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_client_id ON public.orders USING btree (client_id);


--
-- Name: idx_orders_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_deleted_at ON public.orders USING btree (deleted_at);


--
-- Name: idx_orders_shop_hidden_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_shop_hidden_created ON public.orders USING btree (shop_id, is_hidden);


--
-- Name: idx_orders_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_orders_shop_id ON public.orders USING btree (shop_id);


--
-- Name: idx_override_offer_lp; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_override_offer_lp ON public.offer_page_overrides USING btree (offer_id, landing_page_id);


--
-- Name: idx_pixels_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pixels_deleted_at ON public.pixels USING btree (deleted_at);


--
-- Name: idx_plans_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_plans_deleted_at ON public.plans USING btree (deleted_at);


--
-- Name: idx_plans_name; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_plans_name ON public.plans USING btree (name);


--
-- Name: idx_product_images_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_product_images_deleted_at ON public.product_images USING btree (deleted_at);


--
-- Name: idx_product_variant_combinations_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_product_variant_combinations_deleted_at ON public.product_variant_combinations USING btree (deleted_at);


--
-- Name: idx_product_variant_combinations_option1_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_product_variant_combinations_option1_id ON public.product_variant_combinations USING btree (option1_id);


--
-- Name: idx_product_variant_combinations_option2_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_product_variant_combinations_option2_id ON public.product_variant_combinations USING btree (option2_id);


--
-- Name: idx_product_variant_combinations_option3_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_product_variant_combinations_option3_id ON public.product_variant_combinations USING btree (option3_id);


--
-- Name: idx_product_variant_combinations_product_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_product_variant_combinations_product_id ON public.product_variant_combinations USING btree (product_id);


--
-- Name: idx_product_variant_combinations_sku; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_product_variant_combinations_sku ON public.product_variant_combinations USING btree (sku);


--
-- Name: idx_products_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_products_deleted_at ON public.products USING btree (deleted_at);


--
-- Name: idx_shop_available_delivery_company; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_available_delivery_company ON public.delivery_companies USING btree (shop_id, available_delivery_company_id);


--
-- Name: idx_shop_day_visitor; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_day_visitor ON public.shop_visits USING btree (shop_id, day, visitor_id);


--
-- Name: idx_shop_logo_images_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shop_logo_images_deleted_at ON public.shop_logo_images USING btree (deleted_at);


--
-- Name: idx_shop_member_permissions_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shop_member_permissions_deleted_at ON public.shop_member_permissions USING btree (deleted_at);


--
-- Name: idx_shop_members_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shop_members_deleted_at ON public.shop_members USING btree (deleted_at);


--
-- Name: idx_shop_members_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shop_members_shop_id ON public.shop_members USING btree (shop_id);


--
-- Name: idx_shop_members_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shop_members_user_id ON public.shop_members USING btree (user_id);


--
-- Name: idx_shop_phone; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_phone ON public.clients USING btree (shop_id, phone_number);


--
-- Name: idx_shop_platform_pixel; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_platform_pixel ON public.pixels USING btree (shop_id, platform, pixel_id);


--
-- Name: idx_shop_subscriptions_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shop_subscriptions_deleted_at ON public.shop_subscriptions USING btree (deleted_at);


--
-- Name: idx_shop_subscriptions_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_subscriptions_shop_id ON public.shop_subscriptions USING btree (shop_id);


--
-- Name: idx_shop_user; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_user ON public.shop_members USING btree (shop_id, user_id);


--
-- Name: idx_shop_wilaya; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shop_wilaya ON public.delivery_rates USING btree (shop_id, wilaya_id);


--
-- Name: idx_shops_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shops_deleted_at ON public.shops USING btree (deleted_at);


--
-- Name: idx_shops_owner_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_shops_owner_id ON public.shops USING btree (owner_id);


--
-- Name: idx_shops_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_shops_slug ON public.shops USING btree (slug);


--
-- Name: idx_support_ticket_messages_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_ticket_messages_deleted_at ON public.support_ticket_messages USING btree (deleted_at);


--
-- Name: idx_support_ticket_messages_ticket_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_ticket_messages_ticket_id ON public.support_ticket_messages USING btree (ticket_id);


--
-- Name: idx_support_tickets_assigned_to_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_tickets_assigned_to_user_id ON public.support_tickets USING btree (assigned_to_user_id);


--
-- Name: idx_support_tickets_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_tickets_deleted_at ON public.support_tickets USING btree (deleted_at);


--
-- Name: idx_support_tickets_requester_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_tickets_requester_user_id ON public.support_tickets USING btree (requester_user_id);


--
-- Name: idx_support_tickets_shop_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_tickets_shop_id ON public.support_tickets USING btree (shop_id);


--
-- Name: idx_support_tickets_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_support_tickets_status ON public.support_tickets USING btree (status);


--
-- Name: idx_users_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_deleted_at ON public.users USING btree (deleted_at);


--
-- Name: idx_variant_items_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_variant_items_deleted_at ON public.variant_items USING btree (deleted_at);


--
-- Name: idx_variants_deleted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_variants_deleted_at ON public.variants USING btree (deleted_at);


--
-- Name: available_delivery_company_images fk_available_delivery_companies_image; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.available_delivery_company_images
    ADD CONSTRAINT fk_available_delivery_companies_image FOREIGN KEY (available_delivery_company_id) REFERENCES public.available_delivery_companies(id);


--
-- Name: orders fk_clients_orders; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_clients_orders FOREIGN KEY (client_id) REFERENCES public.clients(id) ON DELETE CASCADE;


--
-- Name: coupon_landing_pages fk_coupon_landing_pages_coupon; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_landing_pages
    ADD CONSTRAINT fk_coupon_landing_pages_coupon FOREIGN KEY (coupon_id) REFERENCES public.coupons(id);


--
-- Name: coupon_landing_pages fk_coupon_landing_pages_landing_page; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_landing_pages
    ADD CONSTRAINT fk_coupon_landing_pages_landing_page FOREIGN KEY (landing_page_id) REFERENCES public.landing_pages(id);


--
-- Name: coupon_products fk_coupon_products_coupon; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_products
    ADD CONSTRAINT fk_coupon_products_coupon FOREIGN KEY (coupon_id) REFERENCES public.coupons(id);


--
-- Name: coupon_products fk_coupon_products_product; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.coupon_products
    ADD CONSTRAINT fk_coupon_products_product FOREIGN KEY (product_id) REFERENCES public.products(id);


--
-- Name: delivery_companies fk_delivery_companies_available_delivery_company; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_companies
    ADD CONSTRAINT fk_delivery_companies_available_delivery_company FOREIGN KEY (available_delivery_company_id) REFERENCES public.available_delivery_companies(id);


--
-- Name: delivery_rates fk_delivery_rates_shop; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.delivery_rates
    ADD CONSTRAINT fk_delivery_rates_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id);


--
-- Name: landing_page_images fk_landing_pages_images; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.landing_page_images
    ADD CONSTRAINT fk_landing_pages_images FOREIGN KEY (landing_page_id) REFERENCES public.landing_pages(id) ON DELETE CASCADE;


--
-- Name: landing_pages fk_landing_pages_product; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.landing_pages
    ADD CONSTRAINT fk_landing_pages_product FOREIGN KEY (product_id) REFERENCES public.products(id);


--
-- Name: landing_pages fk_landing_pages_shop; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.landing_pages
    ADD CONSTRAINT fk_landing_pages_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id);


--
-- Name: offer_events fk_offer_events_offer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offer_events
    ADD CONSTRAINT fk_offer_events_offer FOREIGN KEY (offer_id) REFERENCES public.offers(id) ON DELETE CASCADE;


--
-- Name: offer_page_overrides fk_offer_page_overrides_landing_page; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offer_page_overrides
    ADD CONSTRAINT fk_offer_page_overrides_landing_page FOREIGN KEY (landing_page_id) REFERENCES public.landing_pages(id) ON DELETE CASCADE;


--
-- Name: offer_page_overrides fk_offer_page_overrides_offer; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offer_page_overrides
    ADD CONSTRAINT fk_offer_page_overrides_offer FOREIGN KEY (offer_id) REFERENCES public.offers(id) ON DELETE CASCADE;


--
-- Name: offers fk_offers_landing_page; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT fk_offers_landing_page FOREIGN KEY (landing_page_id) REFERENCES public.landing_pages(id) ON DELETE CASCADE;


--
-- Name: offers fk_offers_offer_product; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT fk_offers_offer_product FOREIGN KEY (offer_product_id) REFERENCES public.products(id) ON DELETE CASCADE;


--
-- Name: offers fk_offers_trigger_product; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT fk_offers_trigger_product FOREIGN KEY (trigger_product_id) REFERENCES public.products(id) ON DELETE CASCADE;


--
-- Name: order_items fk_order_items_product; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT fk_order_items_product FOREIGN KEY (product_id) REFERENCES public.products(id);


--
-- Name: order_items fk_order_items_product_variant_combination; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT fk_order_items_product_variant_combination FOREIGN KEY (product_variant_combination_id) REFERENCES public.product_variant_combinations(id) ON DELETE RESTRICT;


--
-- Name: orders fk_orders_client; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_orders_client FOREIGN KEY (client_id) REFERENCES public.clients(id) ON DELETE CASCADE;


--
-- Name: order_items fk_orders_items; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT fk_orders_items FOREIGN KEY (order_id) REFERENCES public.orders(id) ON DELETE CASCADE;


--
-- Name: orders fk_orders_shipped_via; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_orders_shipped_via FOREIGN KEY (shipped_via_id) REFERENCES public.available_delivery_companies(id);


--
-- Name: product_variant_combinations fk_product_variant_combinations_option1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_variant_combinations
    ADD CONSTRAINT fk_product_variant_combinations_option1 FOREIGN KEY (option1_id) REFERENCES public.variant_items(id);


--
-- Name: product_variant_combinations fk_product_variant_combinations_option2; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_variant_combinations
    ADD CONSTRAINT fk_product_variant_combinations_option2 FOREIGN KEY (option2_id) REFERENCES public.variant_items(id);


--
-- Name: product_variant_combinations fk_product_variant_combinations_option3; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_variant_combinations
    ADD CONSTRAINT fk_product_variant_combinations_option3 FOREIGN KEY (option3_id) REFERENCES public.variant_items(id);


--
-- Name: product_variant_combinations fk_products_combinations; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_variant_combinations
    ADD CONSTRAINT fk_products_combinations FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE CASCADE;


--
-- Name: product_images fk_products_images; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_images
    ADD CONSTRAINT fk_products_images FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE CASCADE;


--
-- Name: variants fk_products_variants; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variants
    ADD CONSTRAINT fk_products_variants FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE CASCADE;


--
-- Name: shop_member_permissions fk_shop_member_permissions_action_ref; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_member_permissions
    ADD CONSTRAINT fk_shop_member_permissions_action_ref FOREIGN KEY (action) REFERENCES public.permission_actions(name);


--
-- Name: shop_member_permissions fk_shop_member_permissions_shop_member; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_member_permissions
    ADD CONSTRAINT fk_shop_member_permissions_shop_member FOREIGN KEY (shop_member_id) REFERENCES public.shop_members(id) ON DELETE CASCADE;


--
-- Name: shop_subscriptions fk_shop_subscriptions_plan; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_subscriptions
    ADD CONSTRAINT fk_shop_subscriptions_plan FOREIGN KEY (plan_id) REFERENCES public.plans(id);


--
-- Name: clients fk_shops_clients; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.clients
    ADD CONSTRAINT fk_shops_clients FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;


--
-- Name: shop_logo_images fk_shops_logo_image; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_logo_images
    ADD CONSTRAINT fk_shops_logo_image FOREIGN KEY (shop_id) REFERENCES public.shops(id);


--
-- Name: shop_members fk_shops_members; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_members
    ADD CONSTRAINT fk_shops_members FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;


--
-- Name: orders fk_shops_orders; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_shops_orders FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;


--
-- Name: shops fk_shops_owner; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shops
    ADD CONSTRAINT fk_shops_owner FOREIGN KEY (owner_id) REFERENCES public.users(id);


--
-- Name: pixels fk_shops_pixels; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pixels
    ADD CONSTRAINT fk_shops_pixels FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;


--
-- Name: products fk_shops_products; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.products
    ADD CONSTRAINT fk_shops_products FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;


--
-- Name: support_ticket_messages fk_support_ticket_messages_author; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_ticket_messages
    ADD CONSTRAINT fk_support_ticket_messages_author FOREIGN KEY (author_user_id) REFERENCES public.users(id);


--
-- Name: support_tickets fk_support_tickets_assigned_to; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_tickets
    ADD CONSTRAINT fk_support_tickets_assigned_to FOREIGN KEY (assigned_to_user_id) REFERENCES public.users(id);


--
-- Name: support_ticket_messages fk_support_tickets_messages; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_ticket_messages
    ADD CONSTRAINT fk_support_tickets_messages FOREIGN KEY (ticket_id) REFERENCES public.support_tickets(id) ON DELETE CASCADE;


--
-- Name: support_tickets fk_support_tickets_requester; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_tickets
    ADD CONSTRAINT fk_support_tickets_requester FOREIGN KEY (requester_user_id) REFERENCES public.users(id);


--
-- Name: support_tickets fk_support_tickets_shop; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.support_tickets
    ADD CONSTRAINT fk_support_tickets_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id);


--
-- Name: shop_members fk_users_memberships; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shop_members
    ADD CONSTRAINT fk_users_memberships FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: variant_items fk_variants_variant_items; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.variant_items
    ADD CONSTRAINT fk_variants_variant_items FOREIGN KEY (variant_id) REFERENCES public.variants(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--



-- +goose Down
-- ponytail: no down for the baseline — it's the DB's starting point, not a
-- reversible step. Roll back via a Railway snapshot restore if ever needed.
SELECT 1;
