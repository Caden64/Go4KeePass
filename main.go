package main

import (
    "bufio"
    "fmt"
    "os"
    "os/exec"
    "strings"
    "sync"
    "sync/atomic"
    "time"
)

const workerCount = 10 // Number of concurrent workers

func main() {
    if len(os.Args) != 3 {
        fmt.Printf("Usage: %s <kdbx-file> <wordlist>\n", os.Args[0])
        os.Exit(1)
    }

    kdbxFile := os.Args[1]
    wordlistFile := os.Args[2]

    // Check if keepassxc-cli is installed
    if _, err := exec.LookPath("keepassxc-cli"); err != nil {
        fmt.Println("Error: keepassxc-cli not found in PATH.")
        os.Exit(1)
    }

    // Open the wordlist file
    file, err := os.Open(wordlistFile)
    if err != nil {
        fmt.Printf("Error opening wordlist file: %v\n", err)
        os.Exit(1)
    }
    defer file.Close()

    // Read all passwords into a slice
    var passwords []string
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        passwords = append(passwords, scanner.Text())
    }
    if err := scanner.Err(); err != nil {
        fmt.Printf("Error reading wordlist file: %v\n", err)
        os.Exit(1)
    }

    total := len(passwords)
    fmt.Printf("Total passwords to try: %d\n", total)

    // Channels for communication
    passwordChan := make(chan string, workerCount)
    resultChan := make(chan string)
    doneChan := make(chan struct{})

    var wg sync.WaitGroup
    var tested int64
    startTime := time.Now()

    // Start worker goroutines
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for {
                select {
                case password, ok := <-passwordChan:
                    if !ok {
                        return
                    }
                    // Check if password is correct
                    cmd := exec.Command("keepassxc-cli", "open", "-q", kdbxFile)
                    cmd.Stdin = strings.NewReader(password + "\n")
                    if err := cmd.Run(); err == nil {
                        // Password found
                        select {
                        case resultChan <- password:
                        default:
                        }
                        return
                    }

                    // Update progress
                    currentTested := atomic.AddInt64(&tested, 1)
                    elapsed := time.Since(startTime)
                    attemptsPerMinute := float64(currentTested) / elapsed.Minutes()
                    remaining := total - int(currentTested)
                    estimatedSeconds := int(float64(remaining) / attemptsPerMinute * 60)
                    estimatedTime := formatDuration(estimatedSeconds)
                    fmt.Printf("\rTested: %d/%d | Attempts/min: %.2f | Estimated time remaining: %s", currentTested, total, attemptsPerMinute, estimatedTime)
                case <-doneChan:
                    return
                }
            }
        }()
    }

    // Feed passwords to workers
    go func() {
        for _, password := range passwords {
            select {
            case <-doneChan:
                close(passwordChan)
                return
            default:
                passwordChan <- password
            }
        }
        close(passwordChan)
    }()

    // Wait for result or completion
    select {
    case password := <-resultChan:
        fmt.Printf("\n[*] Password found: %s\n", password)
        close(doneChan)
    case <-time.After(time.Duration(len(passwords)) * time.Second): // Adjust timeout as needed
        fmt.Println("\n[!] Wordlist exhausted, no match found.")
        close(doneChan)
    }

    wg.Wait()
}

// formatDuration formats the duration in weeks, days, hours, minutes, and seconds
func formatDuration(seconds int) string {
    weeks := seconds / (7 * 24 * 3600)
    seconds %= 7 * 24 * 3600
    days := seconds / (24 * 3600)
    seconds %= 24 * 3600
    hours := seconds / 3600
    seconds %= 3600
    minutes := seconds / 60
    seconds %= 60

    var parts []string
    if weeks > 0 {
        parts = append(parts, fmt.Sprintf("%d weeks", weeks))
    }
    if days > 0 {
        parts = append(parts, fmt.Sprintf("%d days", days))
    }
    if hours > 0 {
        parts = append(parts, fmt.Sprintf("%d hours", hours))
    }
    if minutes > 0 {
        parts = append(parts, fmt.Sprintf("%d minutes", minutes))
    }
    if seconds > 0 {
        parts = append(parts, fmt.Sprintf("%d seconds", seconds))
    }

    if len(parts) == 0 {
        return "0 seconds"
    }

    return strings.Join(parts, ", ")
}
