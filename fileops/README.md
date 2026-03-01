# Projet GO — FileOps / WebOps / SecureOps

Projet individuel M1 DevOps.

Outil en ligne de commande développé en Go permettant :
- l’analyse de fichiers texte
- le scraping Wikipédia
- la gestion de sécurité sur fichiers (lock / unlock)

---

## Lancement du programme

Exécution simple :
go run main.go


Avec fichier de configuration :


go run main.go --config config.json


---

## Structure du projet


data/ fichiers d’entrée (.txt)
out/ fichiers générés
config.json configuration du programme
main.go code source


---

## Fonctionnalités

### A — Analyse fichier
- informations fichier (taille, date, lignes)
- statistiques de mots (hors numériques)
- recherche par mot-clé
- filtrage des lignes
- génération :
  - out/filtered.txt
  - out/filtered_not.txt
  - out/head.txt
  - out/tail.txt

---

### B — Analyse multi-fichiers
- analyse de tous les fichiers `.txt` d’un dossier
- génération :
  - out/index.txt
  - out/report.txt
  - out/merged.txt

---

### C — WebOps (Wikipédia)
- téléchargement d’un article Wikipédia
- extraction des paragraphes
- statistiques et filtrage
- export vers :

out/wiki_<article>.txt

- support de plusieurs articles

---

### E — SecureOps
- verrouillage / déverrouillage de fichier (.lock)
- passage en mode read-only
- restauration écriture

---

## Travail réalisé
Développement d’un outil console Go combinant manipulation de fichiers,
scraping web et sécurisation des accès avec journalisation.