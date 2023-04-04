// env.go: main global environment.
// Processing user input and building main Env struct.

package env

import (
	"dollwipe/network"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"
)

const UID = "0"

type (
	File struct {
		Name    string
		Content []byte
	}

	Header string
)

// Extract extension from filename.
func GetExt(fname string) string {
	for i := len(fname) - 1; i >= 0; i-- {
		if fname[i] == '.' {
			return fname[i:]
		}
	}
	return ""
}

// Gen random filename, save original file's extension.
func (f *File) RandName() string {
	rand.Seed(time.Now().UnixNano())
	var (
		letters  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0987654321"
		size     = rand.Int()%20 + 20
		randname = make([]byte, size)
	)
	for i := 0; i < size; i++ {
		randname[i] = letters[rand.Intn(len(letters))]
	}
	return string(randname) + GetExt(f.Name)
}

// WipeMode
const (
	SINGLE = iota
	SHRAPNEL
	CREATING
)

// AntiCaptcha
const (
	RUCAPTCHA = iota
	OCR
	XCAPTCHA
	ANTICATPCHA
	PASSCODE
	MANUAL
)

// TextMode
const (
	FROM_FILE = iota
	NO_CAPS
	SCHIZO
	FROM_POSTS
	DEFAULT
)

var domains = map[string]bool{
	"hk": true,
	// "life": true,
}

// Block for wipe, sorry.
var banned = map[string]bool{
	"rm":   true,
	"math": true,
	"pr":   true,
	"sci":  true,
}

var (
	useProxy = flag.Bool("proxy", false, "вайпать с проксями.")
	useSage  = flag.Bool("sage", false, "клеить сажу.")
	colorize = flag.Bool("masks", false, "цветовые маски для картинок.")

	wipeMode       = flag.Uint64("mode", SHRAPNEL, "режим вайпа:\n\t0 - один тред\n\t1 - шрапнель\n\t2 - создание")
	wait           = flag.Uint64("wait", 20, "ждём секунд печеньки")
	textMode       = flag.Uint64("text", FROM_FILE, "тексты постов:\n\t0 - брать из файла\n\t1 - без текста\n\t2 - шизобред\n\t3 - из постов\n\t4 - дефолтные")
	antiCaptcha    = flag.Uint64("captcha", OCR, "антикапча:\n\t0 - RuCaptcha\n\t1 - OCR")
	antiCaptchaKey = flag.String("key", "", "ключ API антикапчи.")

	board  = flag.String("board", "b", "доска.")
	thread = flag.Uint64("thread", 0, "ID треда, если вайпаем один тред.")

	files = flag.Uint64("files", 0, "кол-во прикрепляемых файлов.")

	filesPath = flag.String("file-path", "./res/files/", "директория с файлами.")
	capsPath  = flag.String("caption-path", "./res/captions.conf", "файл с текстами постов.")
	proxyPath = flag.String("proxy-path", "./res/proxies.conf", "файл с проксями.")

	threads = flag.Uint64("t", 1, "кол-во потоков.")
	iters   = flag.Uint64("i", 1, "кол-во проходов.")
	timeout = flag.Uint64("T", 0, "перерыв между проходами (сек.)")

	bufsize = flag.Uint64("buffer", 0, "размер буфера каналов.")
	limit   = flag.Uint64("limit", 1, "макс. число ошибок соединения для прокси перед удалением.")
	verbose = flag.Bool("v", false, "доп. логи для отладки.")
	domain  = flag.String("domain", "hk", "зеркало.\n\thk\n\tlife (depricated)")

	initAtOnce = flag.Uint64("I", 1, "кол-во параллельно инициализируемых прокси.")
	sessions   = flag.Uint64("s", 1, "кол-во сессий на одну проксю (подробнее в документации).")
)

var defaultCaptions = []string{
	"ALO YOBA ETO TI?",
	"NET, ON U BABUSHKI EST OLADUSHKI."}

var notImplemented = func(x string) error {
	return fmt.Errorf("%s ещё не реализовано.", x)
}

type Mode struct {
	WipeMode    uint8
	AntiCaptcha uint8
	TextMode    uint8
}

type PostSettings struct {
	Sage         bool
	UseProxy     bool
	Colorize     bool
	FilesPerPost uint8
	Board        string
	Thread       uint64
}

type Content struct {
	Files    []File
	Proxies  []network.Proxy
	Captions []string
}

type WipeSettings struct {
	Threads uint64
	Iters   uint64
	Timeout uint64
}

type Env struct {
	//Metadata
	Mode
	PostSettings
	WipeSettings
	*Content

	Key string

	Logger  chan string        // Global synced logger.
	Filter  chan network.Proxy // Global synced proxy filter.
	Status  chan bool          // True if post send, false if failed.
	Verbose bool

	// How many times proxy can fail HTTP request to captcha before get deleted.
	FailedConnectionsLimit uint64

	Domain string

	// How many web driver instances will be spawned at once.
	InitAtOnce uint64
	// Cookie sessions for one proxy.
	Sessions uint64
}

// Get all files which we can post from dir folder.
// They will be loaded at memory once, then we'll use them for posting without loading again.
// 2 * 10^7 bytes is the size limit for single file.
func getFiles(dir string) ([]File, error) {
	cont, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	var (
		files  []File
		failed = 0
		pred   = func(name string) bool {
			name = strings.ToLower(name)
			return strings.HasSuffix(name, ".jpg") ||
				strings.HasSuffix(name, ".png") ||
				strings.HasSuffix(name, ".jpeg") ||
				strings.HasSuffix(name, ".mp4") ||
				strings.HasSuffix(name, ".webm") ||
				strings.HasSuffix(name, ".gif")
		}
	)
	for _, file := range cont {
		if pred(file.Name()) {
			fname := dir + file.Name()
			cont, err := ioutil.ReadFile(fname)
			if err != nil {
				failed++
				continue
			}
			if len(cont) > 2e7 { // 20MB is the limit.
				log.Printf("%s: размер файла превышает допустимый.", fname)
				failed++
				continue
			}
			files = append(files, File{Name: fname, Content: cont})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s: не нашла подходящие файлы (.png, .mp4, etc.)", dir)
	}
	log.Printf("%d/%d файлов инициализировано.", len(files), len(files)+failed)
	return files, nil
}

func getNSplit(dir, pattern string) ([]string, error) {
	cont, err := ioutil.ReadFile(dir)
	if err != nil {
		return nil, err
	}
	return strings.Split(string(cont), pattern), nil
}

func getCaptions(dir string) ([]string, error) {
	return getNSplit(dir, "\n\n")
}

// Get all valid-formated proxies from dir.
func getProxies(dir string, sessions int) ([]network.Proxy, error) {
	result := make([]network.Proxy, 0)
	proxies, err := getNSplit(dir, "\n")
	if err != nil {
		return result, fmt.Errorf("не смогла прочесть файл с проксями: err = %v", err)
	}
	for _, addr := range proxies {
		proxy, err := getProxy(addr)
		if err != nil {
			log.Printf("%s: %v", addr, err)
			continue
		}
		for i := 0; i < sessions; i++ {
			result = append(result, proxy)
			proxy.SessionId++
		}
	}
	if len(result) == 0 {
		return result, fmt.Errorf("не смогла найти ни одной валидной прокси.")
	}
	return result, nil
}

// Call this before init captions.
func (env *Env) parseFiles(dir string) {
	env.Files = make([]File, 0)
	if env.FilesPerPost != 0 {
		log.Println("инициализирую картинки...")
		cont, err := getFiles(dir)
		if err == nil {
			env.Files = cont
			env.FilesPerPost = uint8(math.Min(float64(len(env.Files)), float64(env.FilesPerPost)))
			return
		}
		log.Println(err)
		log.Println("ошибка инициализации, буду продолжать без использования файлов.")
		env.FilesPerPost = 0
	}
}

// Parse all captions (post's texts) to env.Captions.
func (env *Env) parseCaptions(dir string) {
	switch env.TextMode {
	case NO_CAPS:
		if env.FilesPerPost == 0 {
			log.Println("ошибка, не могу постить без текста и без картинок.")
			os.Exit(1)
		}
		env.Captions = []string{""}
	case DEFAULT:
		log.Println("буду использовать дефолтные тексты.")
		env.Captions = defaultCaptions
	case SCHIZO:
		log.Println("SCHIZO not implemented yet")
		os.Exit(0)
	case FROM_POSTS:
		log.Printf("получаю каталог тредов /%s/...", env.Board)
		caps, err := getPostsTexts(env.Board)
		if err != nil {
			log.Printf("ошибка получения постов: %v", err)
			log.Println("буду использовать дефолтные тексты.")
			env.Captions = defaultCaptions
			return
		}
		env.Captions = caps
	case FROM_FILE:
		log.Println("инициализирую тексты постов...")
		caps, err := getCaptions(dir)
		if err == nil {
			env.Captions = caps
			log.Printf("ok, %d текстов инициализировано.", len(caps))
			return
		}
		log.Println("ошибка инициализации, буду использовать дефолтные тексты.")
		env.Captions = defaultCaptions
	default:
		log.Fatal("неизветсный режим текста постов: %d, фатальная ошибка.", env.TextMode)
	}
}

// Check for validness and parse all proxies to env.Proxies with []network.Proxy type.
func (env *Env) parseProxies(dir string) {
	if env.UseProxy {
		log.Println("инициализирую прокси...")
		proxies, err := getProxies(dir, int(env.Sessions))
		if err != nil {
			log.Println(err)
			log.Fatal("ошибка инициализации, не удалось инициализировать прокси, фатальная ошибка.")
		}
		env.Proxies = proxies
	}
}

func ParseEnv() (*Env, error) {
	flag.Parse()
	log.SetFlags(log.Ltime)

	env := Env{
		Mode: Mode{
			WipeMode:    uint8(*wipeMode),
			AntiCaptcha: uint8(*antiCaptcha),
			TextMode:    uint8(*textMode),
		},
		PostSettings: PostSettings{
			UseProxy:     *useProxy,
			Sage:         *useSage,
			Colorize:     *colorize,
			Thread:       *thread,
			Board:        *board,
			FilesPerPost: uint8(math.Min(float64(*files), 4)),
		},
		WipeSettings: WipeSettings{
			Threads: *threads,
			Iters:   *iters,
			Timeout: *timeout,
		},
		Content: new(Content),
		Key:     *antiCaptchaKey,
		Logger:  make(chan string, *bufsize),
		Filter:  make(chan network.Proxy, *bufsize),
		Status:  make(chan bool, *bufsize),

		FailedConnectionsLimit: *limit,
		Verbose:                *verbose,
		Domain:                 *domain,
		InitAtOnce:             *initAtOnce,
		Sessions:               *sessions,
	}
	if env.InitAtOnce == 0 {
		return nil, fmt.Errorf("ошибка, -init должен быть больше нуля.")
	}
	if env.Sessions == 0 {
		return nil, fmt.Errorf("ошибка, -s должен быть больше нуля.")
	}
	if banned[env.Board] {
		return nil, fmt.Errorf("извини, но эту доску вайпать нельзя, она защищена магическим полем. Такие дела!")
	}
	if env.Domain == "life" {
		return nil, fmt.Errorf("2ch.life support deprecated")
	}
	if _, ok := domains[env.Domain]; !ok {
		return nil, fmt.Errorf("ошибка, не смогла распознать домен зеркала: %s", env.Domain)
	}
	if env.WipeMode == SINGLE && env.Thread == 0 {
		return nil, fmt.Errorf("ошибка, не указан ID треда.")
	}
	if env.WipeMode != SINGLE && env.Thread != 0 {
		return nil, fmt.Errorf("ошибка, ID треда указан, но режим не SingleThread.")
	}
	if env.AntiCaptcha != RUCAPTCHA && env.AntiCaptcha != OCR {
		return nil, notImplemented("антикапча, кроме RuCaptcha или OCR")
	}

	env.parseFiles(*filesPath)
	if env.FilesPerPost == 0 && env.WipeMode == CREATING {
		return nil, fmt.Errorf("для создания тредов нужен хотя бы один файл!")
	}
	env.parseCaptions(*capsPath)
	env.parseProxies(*proxyPath)

	return &env, nil
}
