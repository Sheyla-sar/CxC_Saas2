-- Crea el rol y la base de la aplicación en Neon (o en cualquier PostgreSQL administrado).
-- Ejecútelo en el SQL Editor de Neon, conectado como el rol dueño de su proyecto (por defecto
-- neondb_owner). La aplicación se niega a arrancar con un rol que sea superusuario o tenga
-- BYPASSRLS, porque Row Level Security no lo restringiría: por eso NO use el rol por defecto.
--
-- 1) Cambie CAMBIAR por una contraseña larga sin caracteres especiales:
--      openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c 40
-- 2) Si su rol dueño no se llama neondb_owner, cámbielo en la segunda línea.

CREATE ROLE cxc_app LOGIN PASSWORD 'CAMBIAR' NOSUPERUSER NOBYPASSRLS NOCREATEROLE NOCREATEDB;
GRANT cxc_app TO neondb_owner;
CREATE DATABASE cxc OWNER cxc_app;

-- Después, DATABASE_URL será:
--   postgres://cxc_app:CAMBIAR@<host-SIN-pooler>/cxc?sslmode=require
-- (el host de Neon sin "-pooler"; ver DESPLIEGUE_VERCEL.md §4)
