package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// config lue depuis config.json (defaults si champs vide)
type Config struct {
	DefaultFile string `json:"default_file"`
	BaseDir     string `json:"base_dir"`
	OutDir      string `json:"out_dir"`
	DefaultExt  string `json:"default_ext"`
	WikiLang    string `json:"wiki_lang"`
	ProcessTopN int    `json:"process_top_n"`
}

// Charge config.json + merge avec valeurs par défaut
func loadConfigJSON(path string) Config {
	cfg := Config{
		DefaultFile: "data/input.txt",
		BaseDir:     "data",
		OutDir:      "out",
		DefaultExt:  ".txt",
		WikiLang:    "fr",
		ProcessTopN: 10,
	}

	// lecture du json
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("ERREUR : config.json introuvable. Valeurs par défaut utilisées.")
		return cfg
	}

	// parse json
	var loaded Config
	if err := json.Unmarshal(b, &loaded); err != nil {
		fmt.Println("ERREUR : config.json invalide. Valeurs par défaut utilisées. Détail:", err)
		return cfg
	}

	// merge champ par champ
	if loaded.DefaultFile != "" {
		cfg.DefaultFile = loaded.DefaultFile
	}
	if loaded.BaseDir != "" {
		cfg.BaseDir = loaded.BaseDir
	}
	if loaded.OutDir != "" {
		cfg.OutDir = loaded.OutDir
	}
	if loaded.DefaultExt != "" {
		cfg.DefaultExt = loaded.DefaultExt
	}
	if loaded.WikiLang != "" {
		cfg.WikiLang = loaded.WikiLang
	}
	if loaded.ProcessTopN > 0 {
		cfg.ProcessTopN = loaded.ProcessTopN
	}
	return cfg
}

// quick check fichier
func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// quick check dossier
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// compte lignes (scanner buffer pr gros fichier)
func countLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	lines := 0
	for sc.Scan() {
		lines++
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return lines, nil
}

// ---------- Helpers (mots / keyword) ----------

// détecte un mot digit
func isNumericWord(w string) bool {
	if w == "" {
		return false
	}
	for _, r := range w {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// stats mots depuis txt
func wordStatsFromText(text string) (count int, avg float64) {
	words := strings.Fields(text)
	totalLen := 0
	count = 0

	for _, w := range words {
		w = strings.Trim(w, " \t\r\n.,;:!?\"'()[]{}<>")
		if w == "" {
			continue
		}
		if isNumericWord(w) {
			continue
		}
		count++
		totalLen += len([]rune(w))
	}

	if count == 0 {
		return 0, 0
	}
	return count, float64(totalLen) / float64(count)
}

// stats mots depuis un fichier
func wordStatsFromFile(path string) (count int, avg float64, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	c, a := wordStatsFromText(string(b))
	return c, a, nil
}

func containsKeyword(line, keyword string) bool {
	if keyword == "" {
		return false
	}
	return strings.Contains(strings.ToLower(line), strings.ToLower(keyword))
}

// input int (enter)
func askInt(reader *bufio.Reader, prompt string, defaultVal int) int {
	fmt.Print(prompt)
	s, _ := reader.ReadString('\n')
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// ---------- Choix A (FileOps fichier) ----------

// compte les lignes qui match un mot clé
func countLinesWithKeyword(path, keyword string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	sc := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	count := 0
	for sc.Scan() {
		if containsKeyword(sc.Text(), keyword) {
			count++
		}
	}
	return count, sc.Err()
}

// split lignes
func filterLines(path, keyword, outMatch, outNot string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	f1, err := os.Create(outMatch)
	if err != nil {
		return err
	}
	defer f1.Close()

	f2, err := os.Create(outNot)
	if err != nil {
		return err
	}
	defer f2.Close()

	w1 := bufio.NewWriter(f1)
	w2 := bufio.NewWriter(f2)
	defer w1.Flush()
	defer w2.Flush()

	sc := bufio.NewScanner(in)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := sc.Text()
		if containsKeyword(line, keyword) {
			fmt.Fprintln(w1, line)
		} else {
			fmt.Fprintln(w2, line)
		}
	}
	return sc.Err()
}

// head -> N premieres lignes
func headToFile(path string, n int, outPath string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	defer w.Flush()

	sc := bufio.NewScanner(in)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	i := 0
	for sc.Scan() {
		if i >= n {
			break
		}
		fmt.Fprintln(w, sc.Text())
		i++
	}
	return sc.Err()
}

// tail -> N dernieres lignes
func tailToFile(path string, n int, outPath string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	sc := bufio.NewScanner(in)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	if n <= 0 {
		_, err := os.Create(outPath)
		return err
	}

	ring := make([]string, 0, n)
	for sc.Scan() {
		line := sc.Text()
		if len(ring) < n {
			ring = append(ring, line)
		} else {
			copy(ring, ring[1:])
			ring[n-1] = line
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	defer w.Flush()

	for _, line := range ring {
		fmt.Fprintln(w, line)
	}
	return nil
}

// ---------- Choix B (multi-fichiers) ----------

// info index par fichier
type FileIndexItem struct {
	Path    string
	Size    int64
	ModTime time.Time
	Lines   int
	Words   int
}

// liste *.txt dossier
func listTxtFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(strings.ToLower(name), ".txt") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}

// construction index complet
func buildIndexForDir(dir string) ([]FileIndexItem, error) {
	paths, err := listTxtFiles(dir)
	if err != nil {
		return nil, err
	}

	items := make([]FileIndexItem, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		lines, err := countLines(p)
		if err != nil {
			return nil, err
		}
		words, _, err := wordStatsFromFile(p)
		if err != nil {
			return nil, err
		}

		items = append(items, FileIndexItem{
			Path:    p,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Lines:   lines,
			Words:   words,
		})
	}
	return items, nil
}

// ecrit index.txt
func writeIndexFile(outPath string, items []FileIndexItem) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "INDEX (chemin | taille(bytes) | dernière_modif | lignes | mots_hors_num)")
	for _, it := range items {
		fmt.Fprintf(w, "%s | %d | %s | %d | %d\n",
			it.Path, it.Size, it.ModTime.Format(time.RFC3339), it.Lines, it.Words)
	}
	return nil
}

// report.txt global (total + moyenne)
func writeReportFile(outPath string, items []FileIndexItem) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	totalFiles := len(items)
	var totalSize int64
	totalLines := 0
	totalWords := 0

	for _, it := range items {
		totalSize += it.Size
		totalLines += it.Lines
		totalWords += it.Words
	}

	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "REPORT GLOBAL")
	fmt.Fprintln(w, "------------")
	fmt.Fprintf(w, "Nb fichiers .txt analysés : %d\n", totalFiles)
	fmt.Fprintf(w, "Taille totale (bytes)     : %d\n", totalSize)
	fmt.Fprintf(w, "Total lignes              : %d\n", totalLines)
	fmt.Fprintf(w, "Total mots (hors num)     : %d\n", totalWords)

	if totalFiles > 0 {
		fmt.Fprintf(w, "Moyenne lignes/fichier     : %.2f\n", float64(totalLines)/float64(totalFiles))
		fmt.Fprintf(w, "Moyenne mots/fichier       : %.2f\n", float64(totalWords)/float64(totalFiles))
	}

	fmt.Fprintln(w, "\nDétail par fichier :")
	for _, it := range items {
		fmt.Fprintf(w, "- %s (lignes=%d, mots=%d, taille=%d)\n", it.Path, it.Lines, it.Words, it.Size)
	}

	return nil
}

// fusionne tous les .txt de base_dir vers merged.txt
func mergeTxtFromBaseDir(baseDir, outPath string) error {
	paths, err := listTxtFiles(baseDir)
	if err != nil {
		return err
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	defer w.Flush()

	for _, p := range paths {
		fmt.Fprintf(w, "----- BEGIN %s -----\n", p)

		in, err := os.Open(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		in.Close()
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "\n----- END %s -----\n\n", p)
	}

	return nil
}

// ---------- Choix C (WebOps Wikipedia via goquery) ----------

// safe filename pour éviter / : etc.
func safeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "article"
	}
	repl := strings.NewReplacer(
		" ", "_", "/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	return repl.Replace(s)
}

// recup paragraphes wiki
func fetchWikipediaParagraphsFR(article string) (url string, paragraphs []string, err error) {
	article = strings.TrimSpace(article)
	if article == "" {
		return "", nil, fmt.Errorf("article vide")
	}

	url = "https://fr.wikipedia.org/wiki/" + article

	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return url, nil, err
	}

	//FIX : refus de wiki ?
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GoWikiBot/1.0; +https://example.com)")

	resp, err := client.Do(req)
	if err != nil {
		return url, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return url, nil, fmt.Errorf("HTTP %d sur %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return url, nil, err
	}

	// contenu principal
	doc.Find("#mw-content-text .mw-parser-output > p").Each(func(i int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if t != "" {
			paragraphs = append(paragraphs, t)
		}
	})

	if len(paragraphs) == 0 {
		return url, nil, fmt.Errorf("aucun paragraphe trouvé")
	}

	return url, paragraphs, nil
}

// écrit wiki_article.txt
func writeWikiOutput(outPath, url, article string, paragraphs []string, keyword string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	fullText := strings.Join(paragraphs, "\n\n")
	words, avg := wordStatsFromText(fullText)

	fmt.Fprintln(w, "WIKIPEDIA ANALYSE")
	fmt.Fprintln(w, "----------------")
	fmt.Fprintln(w, "Article :", article)
	fmt.Fprintln(w, "URL     :", url)
	fmt.Fprintln(w, "Nb paragraphes :", len(paragraphs))
	fmt.Fprintln(w, "Nb mots (hors numériques) :", words)
	fmt.Fprintf(w, "Longueur moyenne des mots : %.2f\n", avg)

	kwCount := 0
	var match []string
	var notMatch []string

	if keyword != "" {
		for _, p := range paragraphs {
			if containsKeyword(p, keyword) {
				kwCount++
				match = append(match, p)
			} else {
				notMatch = append(notMatch, p)
			}
		}
		fmt.Fprintln(w, "\nMot-clé :", keyword)
		fmt.Fprintln(w, "Paragraphes contenant le mot-clé :", kwCount)
	} else {
		fmt.Fprintln(w, "\nMot-clé : (vide) -> pas de filtrage")
	}

	fmt.Fprintln(w, "\n--- TEXTE COMPLET (paragraphes) ---\n")
	fmt.Fprintln(w, fullText)

	if keyword != "" {
		fmt.Fprintln(w, "\n--- PARAGRAPHES AVEC MOT-CLÉ ---\n")
		for _, p := range match {
			fmt.Fprintln(w, p)
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, "\n--- PARAGRAPHES SANS MOT-CLÉ ---\n")
		for _, p := range notMatch {
			fmt.Fprintln(w, p)
			fmt.Fprintln(w)
		}
	}

	return nil
}

// ---------- MAIN ----------

func main() {
	// --config
	configPath := flag.String("config", "config.json", "chemin vers le fichier de config JSON")
	flag.Parse()

	cfg := loadConfigJSON(*configPath)
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Programme démarré")

	currentFile := cfg.DefaultFile
	if !isFile(currentFile) {
		fmt.Println("Attention : default_file invalide ou inexistant :", currentFile)
	} else {
		fmt.Println("Fichier courant :", currentFile)
	}

	// menu boucle infinie
	for {
		fmt.Println("\n==== MENU ====")
		fmt.Println("Fichier courant:", currentFile)
		fmt.Println("1 - Changer le fichier courant")
		fmt.Println("A - Choix A (analyse fichier)")
		fmt.Println("B - Choix B (analyse multi-fichiers)")
		fmt.Println("C - Choix C (analyser une page Wikipédia)")
		fmt.Println("E - SecureOps (lock/unlock + read-only + audit)")
		fmt.Println("Q - Quitter")
		fmt.Print("Votre choix : ")

		input, _ := reader.ReadString('\n')
		choice := strings.ToUpper(strings.TrimSpace(input))

		switch choice {
		case "1":
			// change fichier courant
			fmt.Print("Nouveau fichier (ENTER = default_file): ")
			p, _ := reader.ReadString('\n')
			p = strings.TrimSpace(p)
			if p == "" {
				p = cfg.DefaultFile
			}
			if !isFile(p) {
				fmt.Println("Fichier invalide :", p)
				continue
			}
			currentFile = p
			fmt.Println("Nouveau fichier courant :", currentFile)

		case "A":
			// analyse fichier + outputs out
			fmt.Print("Chemin fichier (ENTER = fichier courant) : ")
			p, _ := reader.ReadString('\n')
			p = strings.TrimSpace(p)
			if p == "" {
				p = currentFile
			}
			if !isFile(p) {
				fmt.Println("Fichier invalide :", p)
				continue
			}

			//  si lock => refus ecritures
			if isLocked(cfg, p) {
				fmt.Println("Fichier verrouillé -> opération refusée :", p)
				auditLog(cfg, "DENY_LOCKED", p+" | Choix A")
				continue
			}

			if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
				fmt.Println("Erreur création out/:", err)
				continue
			}

			info, err := os.Stat(p)
			if err != nil {
				fmt.Println("Erreur stat :", err)
				continue
			}

			nbLines, err := countLines(p)
			if err != nil {
				fmt.Println("Erreur countLines :", err)
				continue
			}

			fmt.Print("Mot-clé à chercher : ")
			kw, _ := reader.ReadString('\n')
			kw = strings.TrimSpace(kw)

			n := askInt(reader, "N pour head/tail (ENTER = 10) : ", 10)

			nbWords, avgLen, err := wordStatsFromFile(p)
			if err != nil {
				fmt.Println("Erreur wordStats:", err)
				continue
			}

			linesWithKW := 0
			if kw != "" {
				linesWithKW, err = countLinesWithKeyword(p, kw)
				if err != nil {
					fmt.Println("Erreur countLinesWithKeyword:", err)
					continue
				}
			}

			// outputs
			outFiltered := filepath.Join(cfg.OutDir, "filtered.txt")
			outFilteredNot := filepath.Join(cfg.OutDir, "filtered_not.txt")
			outHead := filepath.Join(cfg.OutDir, "head.txt")
			outTail := filepath.Join(cfg.OutDir, "tail.txt")

			if kw != "" {
				if err := filterLines(p, kw, outFiltered, outFilteredNot); err != nil {
					fmt.Println("Erreur filterLines:", err)
					continue
				}
			} else {
				_ = os.WriteFile(outFiltered, []byte(""), 0644)
				_ = os.WriteFile(outFilteredNot, []byte(""), 0644)
			}

			if err := headToFile(p, n, outHead); err != nil {
				fmt.Println("Erreur headToFile:", err)
				continue
			}
			if err := tailToFile(p, n, outTail); err != nil {
				fmt.Println("Erreur tailToFile:", err)
				continue
			}

			fmt.Println("\n--- Infos fichier ---")
			fmt.Println("Chemin         :", p)
			fmt.Println("Taille (bytes) :", info.Size())
			fmt.Println("Création       : N/A (non portable en Go standard)")
			fmt.Println("Dernière modif :", info.ModTime().Format(time.RFC3339))
			fmt.Println("Nb lignes      :", nbLines)

			fmt.Println("\n--- Stats mots ---")
			fmt.Println("Nb mots (hors numériques) :", nbWords)
			fmt.Printf("Longueur moyenne          : %.2f\n", avgLen)

			fmt.Println("\n--- Mot-clé ---")
			if kw == "" {
				fmt.Println("Mot-clé : (vide) -> skip comptage/filtrage")
			} else {
				fmt.Println("Mot-clé :", kw)
				fmt.Println("Lignes contenant le mot-clé :", linesWithKW)
			}

			fmt.Println("\n--- Fichiers générés ---")
			fmt.Println(outFiltered)
			fmt.Println(outFilteredNot)
			fmt.Println(outHead)
			fmt.Println(outTail)

		case "B":
			// analyse batch + index merge
			fmt.Print("Répertoire à analyser (ENTER = base_dir) : ")
			dir, _ := reader.ReadString('\n')
			dir = strings.TrimSpace(dir)
			if dir == "" {
				dir = cfg.BaseDir
			}

			if !isDir(dir) {
				fmt.Println("Répertoire invalide :", dir)
				continue
			}

			if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
				fmt.Println("Erreur création out/:", err)
				continue
			}

			items, err := buildIndexForDir(dir)
			if err != nil {
				fmt.Println("Erreur analyse répertoire:", err)
				continue
			}

			outIndex := filepath.Join(cfg.OutDir, "index.txt")
			if err := writeIndexFile(outIndex, items); err != nil {
				fmt.Println("Erreur writeIndexFile:", err)
				continue
			}

			outReport := filepath.Join(cfg.OutDir, "report.txt")
			if err := writeReportFile(outReport, items); err != nil {
				fmt.Println("Erreur writeReportFile:", err)
				continue
			}

			outMerged := filepath.Join(cfg.OutDir, "merged.txt")
			if err := mergeTxtFromBaseDir(cfg.BaseDir, outMerged); err != nil {
				fmt.Println("Erreur merge:", err)
				continue
			}

			fmt.Println("\n--- Choix B terminé ---")
			fmt.Println("Fichiers générés :")
			fmt.Println(outIndex)
			fmt.Println(outReport)
			fmt.Println(outMerged)

		case "C":
			// wiki : recup + traitement
			if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
				fmt.Println("Erreur création out/:", err)
				continue
			}

			fmt.Print("Article Wikipédia (ex: Go_(langage))\nTu peux en mettre plusieurs séparés par virgule : ")
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				fmt.Println("Article vide.")
				continue
			}

			fmt.Print("Mot-clé à chercher dans les paragraphes (ENTER = aucun) : ")
			kw, _ := reader.ReadString('\n')
			kw = strings.TrimSpace(kw)

			articles := strings.Split(line, ",")
			for _, a := range articles {
				article := strings.TrimSpace(a)
				if article == "" {
					continue
				}

				fmt.Println("\nTéléchargement :", article)

				url, paragraphs, err := fetchWikipediaParagraphsFR(article)
				if err != nil {
					fmt.Println("Erreur Wikipedia:", err)
					continue
				}

				outPath := filepath.Join(cfg.OutDir, "wiki_"+safeFileName(article)+".txt")
				if err := writeWikiOutput(outPath, url, article, paragraphs, kw); err != nil {
					fmt.Println("Erreur écriture:", err)
					continue
				}

				fmt.Println("OK ->", outPath)
			}

		case "E":
			//   lock/unlock + read-only + audit.log
			if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
				fmt.Println("Erreur création out/:", err)
				continue
			}

			fmt.Println("\n--- SecureOps ---")
			fmt.Println("1 - Verrouiller un fichier (lockfile)")
			fmt.Println("2 - Déverrouiller un fichier (lockfile)")
			fmt.Println("3 - Passer un fichier en READ-ONLY")
			fmt.Println("4 - Remettre un fichier en WRITE (annuler read-only)")
			fmt.Print("Votre choix : ")

			sub, _ := reader.ReadString('\n')
			sub = strings.TrimSpace(sub)

			fmt.Print("Chemin fichier (ENTER = fichier courant) : ")
			p, _ := reader.ReadString('\n')
			p = strings.TrimSpace(p)
			if p == "" {
				p = currentFile
			}
			if !isFile(p) {
				fmt.Println("Fichier invalide :", p)
				continue
			}

			switch sub {
			case "1":
				// lock
				if !confirm(reader, "Confirmer verrouillage") {
					fmt.Println("Annulé.")
					continue
				}
				if err := lockFile(cfg, p); err != nil {
					fmt.Println("Erreur lock:", err)
					auditLog(cfg, "LOCK_FAIL", p+" | "+err.Error())
					continue
				}
				fmt.Println("OK verrouillé ->", lockPath(cfg, p))
				auditLog(cfg, "LOCK", p)

			case "2":
				// unlock
				if !confirm(reader, "Confirmer déverrouillage") {
					fmt.Println("Annulé.")
					continue
				}
				if err := unlockFile(cfg, p); err != nil {
					fmt.Println("Erreur unlock:", err)
					auditLog(cfg, "UNLOCK_FAIL", p+" | "+err.Error())
					continue
				}
				fmt.Println("OK déverrouillé")
				auditLog(cfg, "UNLOCK", p)

			case "3":
				// read-only
				if !confirm(reader, "Confirmer passage en read-only") {
					fmt.Println("Annulé.")
					continue
				}
				if err := setReadOnly(p, true); err != nil {
					fmt.Println("Erreur read-only:", err)
					auditLog(cfg, "READONLY_FAIL", p+" | "+err.Error())
					continue
				}
				fmt.Println("OK -> fichier en read-only")
				auditLog(cfg, "READONLY_ON", p)

			case "4":
				// write back
				if !confirm(reader, "Confirmer remise en écriture") {
					fmt.Println("Annulé.")
					continue
				}
				if err := setReadOnly(p, false); err != nil {
					fmt.Println("Erreur write:", err)
					auditLog(cfg, "READONLY_OFF_FAIL", p+" | "+err.Error())
					continue
				}
				fmt.Println("OK -> fichier réinscriptible")
				auditLog(cfg, "READONLY_OFF", p)

			default:
				fmt.Println("Choix invalide SecureOps")
			}

		case "Q":
			fmt.Println("Bye !")
			return

		default:
			fmt.Println("Choix invalide")
		}
	}
}

// --------- Helpers Lock/Unlock --------

// path lockfile
func lockPath(cfg Config, target string) string {
	base := safeFileName(filepath.Base(target))
	return filepath.Join(cfg.OutDir, base+".lock")
}

// fichier .lock présent
func isLocked(cfg Config, target string) bool {
	_, err := os.Stat(lockPath(cfg, target))
	return err == nil
}

// creation du lock
func lockFile(cfg Config, target string) error {
	lp := lockPath(cfg, target)
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("déjà verrouillé (lock existe): %s", lp)
		}
		return err
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format(time.RFC3339) + " locked " + target + "\n")
	return nil
}

// unlock: suppr lock
func unlockFile(cfg Config, target string) error {
	lp := lockPath(cfg, target)
	if _, err := os.Stat(lp); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pas verrouillé (lock absent): %s", lp)
		}
		return err
	}
	return os.Remove(lp)
}

// confirmation yes no
func confirm(reader *bufio.Reader, prompt string) bool {
	fmt.Print(prompt + " (yes/no): ")
	s, _ := reader.ReadString('\n')
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "yes" || s == "y"
}

// audit log
func auditLog(cfg Config, action, details string) {
	_ = os.MkdirAll(cfg.OutDir, 0755)
	path := filepath.Join(cfg.OutDir, "audit.log")

	line := fmt.Sprintf("%s | %s | %s\n", time.Now().Format(time.RFC3339), action, details)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("WARN audit.log:", err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// read-only chmod / attrib
func setReadOnly(target string, readOnly bool) error {
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cible invalide: c'est un répertoire")
	}

	if runtime.GOOS == "windows" {
		// windows: attrib +R / -R
		attr := "-R"
		if readOnly {
			attr = "+R"
		}
		cmd := exec.Command("attrib", attr, target)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("attrib error: %v (%s)", err, string(out))
		}
		return nil
	}

	//  chmod pr mac/linux
	if readOnly {
		return os.Chmod(target, 0444)
	}
	return os.Chmod(target, 0644)
}
