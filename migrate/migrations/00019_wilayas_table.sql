-- +goose Up
-- Moves the 58 Algerian wilayas out of the embedded static_wilayas.json
-- (server/initializers/data/static_wilayas.json, read via go:embed) and into
-- a real table, per root CLAUDE.md's "never hardcode wilaya/city lists".
-- Seeded verbatim from that JSON's current contents. Runtime now reads this
-- table (services.GetWilayas, cached in-memory) instead of the embedded
-- file — see server/initializers/wilayas.go and server/services/wilayas.go.
-- Fixed set of 58: Algeria isn't getting new provinces, so this is edit-only,
-- no create/delete, same shape as public.shop_roles.
CREATE TABLE public.wilayas (
    id integer NOT NULL,
    name text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    has_stopdesk boolean NOT NULL DEFAULT false,
    stopdesk_rate double precision NOT NULL DEFAULT 0,
    has_doorstep boolean NOT NULL DEFAULT false,
    doorstep_rate double precision NOT NULL DEFAULT 0
);

ALTER TABLE ONLY public.wilayas
    ADD CONSTRAINT wilayas_pkey PRIMARY KEY (id);

INSERT INTO public.wilayas (id, name, is_active, has_stopdesk, stopdesk_rate, has_doorstep, doorstep_rate) VALUES
    (1,  'Adrar',               true,  true,  1000, true,  1150),
    (2,  'Chlef',                true,  true,  550,  true,  700),
    (3,  'Laghouat',             true,  true,  650,  true,  800),
    (4,  'Oum El Bouaghi',       true,  true,  650,  true,  800),
    (5,  'Batna',                true,  true,  600,  true,  750),
    (6,  'Bejaia',               true,  true,  550,  true,  700),
    (7,  'Biskra',               true,  true,  650,  true,  800),
    (8,  'Bechar',               true,  true,  900,  true,  1050),
    (9,  'Blida',                true,  true,  450,  true,  600),
    (10, 'Bouira',               true,  true,  500,  true,  650),
    (11, 'Tamanrasset',          true,  true,  1200, true,  1350),
    (12, 'Tebessa',              true,  true,  700,  true,  850),
    (13, 'Tlemcen',              true,  true,  700,  true,  850),
    (14, 'Tiaret',               true,  true,  550,  true,  700),
    (15, 'Tizi Ouzou',           true,  true,  500,  true,  650),
    (16, 'Alger',                true,  true,  400,  true,  500),
    (17, 'Djelfa',               true,  true,  600,  true,  750),
    (18, 'Jijel',                true,  true,  600,  true,  750),
    (19, 'Setif',                true,  true,  550,  true,  700),
    (20, 'Saida',                true,  true,  650,  true,  800),
    (21, 'Skikda',               true,  true,  600,  true,  800),
    (22, 'Sidi Bel Abbes',       true,  true,  650,  true,  800),
    (23, 'Annaba',               true,  true,  700,  true,  850),
    (24, 'Guelma',               true,  true,  650,  true,  800),
    (25, 'Constantine',          true,  true,  600,  true,  750),
    (26, 'Medea',                true,  true,  450,  true,  600),
    (27, 'Mostaganem',           true,  true,  600,  true,  750),
    (28, 'M''Sila',              true,  true,  550,  true,  700),
    (29, 'Mascara',              true,  true,  600,  true,  750),
    (30, 'Ouargla',              true,  true,  800,  true,  950),
    (31, 'Oran',                 true,  true,  600,  true,  800),
    (32, 'El Bayadh',            true,  true,  650,  true,  800),
    (33, 'Illizi',                false, false, 0,    false, 0),
    (34, 'Bordj Bou Arreridj',   true,  true,  550,  true,  700),
    (35, 'Boumerdes',            true,  true,  450,  true,  600),
    (36, 'El Tarf',              true,  true,  700,  true,  850),
    (37, 'Tindouf',              true,  true,  1150, true,  1300),
    (38, 'Tissemsilt',           true,  true,  550,  true,  700),
    (39, 'El Oued',              true,  true,  750,  true,  900),
    (40, 'Khenchela',            true,  true,  650,  true,  800),
    (41, 'Souk Ahras',           true,  true,  700,  true,  850),
    (42, 'Tipaza',               true,  true,  450,  true,  600),
    (43, 'Mila',                 true,  true,  600,  true,  750),
    (44, 'Ain Defla',            true,  true,  500,  true,  650),
    (45, 'Naama',                true,  true,  750,  true,  900),
    (46, 'Ain Temouchent',       true,  true,  650,  true,  800),
    (47, 'Ghardaia',             true,  true,  750,  true,  900),
    (48, 'Relizane',             true,  true,  600,  true,  750),
    (49, 'Timimoun',             true,  true,  950,  true,  1100),
    (50, 'Bordj Badji Mokhtar',  false, false, 0,    false, 0),
    (51, 'Ouled Djellal',        true,  true,  650,  true,  800),
    (52, 'Béni Abbès',           true,  true,  950,  true,  1100),
    (53, 'In Salah',             true,  true,  950,  true,  1100),
    (54, 'In Guezzam',           false, false, 0,    false, 0),
    (55, 'Touggourt',            true,  true,  750,  true,  900),
    (56, 'Djanet',                false, false, 0,    false, 0),
    (57, 'M''Ghair',             true,  true,  700,  true,  850),
    (58, 'Meniaa',               true,  true,  850,  true,  1000);

-- +goose Down
DROP TABLE IF EXISTS public.wilayas;
