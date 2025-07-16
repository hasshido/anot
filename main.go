package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

// CIDRMatcher pre-parses CIDR ranges for efficient matching
type CIDRMatcher struct {
	networks []*net.IPNet
}

// NewCIDRMatcher creates a new CIDR matcher with pre-parsed networks
func NewCIDRMatcher(cidrs []string) *CIDRMatcher {
	matcher := &CIDRMatcher{}
	for _, cidr := range cidrs {
		if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
			matcher.networks = append(matcher.networks, ipNet)
		}
	}
	return matcher
}

// Contains checks if an IP is contained in any of the CIDR ranges
func (c *CIDRMatcher) Contains(ip net.IP) bool {
	for _, network := range c.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// matchesWildcard checks if a line matches a wildcard pattern
// For example: "*.customer.cloudways.com" matches "test1.customer.cloudways.com" but not "customer.cloudways.com"
func matchesWildcard(line, pattern string) bool {
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	
	// Remove the "*" from the pattern to get the suffix
	suffix := pattern[1:] // Remove the "*" but keep the "."
	
	// The line must end with the suffix
	if !strings.HasSuffix(line, suffix) {
		return false
	}
	
	// The line must be longer than the suffix (to ensure there's a subdomain)
	if len(line) <= len(suffix) {
		return false
	}
	
	// The part before the suffix should not contain any dots at the end
	// This ensures *.example.com matches sub.example.com but not example.com
	beforeSuffix := line[:len(line)-len(suffix)]
	return len(beforeSuffix) > 0 && !strings.HasSuffix(beforeSuffix, ".")
}

// shouldRemoveLine checks if a line should be removed based on the removal patterns
// This optimized version minimizes repeated parsing and uses pre-compiled matchers
func shouldRemoveLine(line string, exactMatches map[string]bool, wildcardPatterns []string, cidrMatcher *CIDRMatcher) bool {
	// Check for exact match first (fastest lookup)
	if exactMatches[line] {
		return true
	}
	
	// Parse IP once and check CIDR ranges if it's a valid IP
	if ip := net.ParseIP(line); ip != nil && cidrMatcher != nil {
		if cidrMatcher.Contains(ip) {
			return true
		}
	}
	
	// Check wildcard patterns (only for non-IP strings to avoid unnecessary work)
	if !strings.Contains(line, ":") && !isNumericIP(line) {
		for _, pattern := range wildcardPatterns {
			if matchesWildcard(line, pattern) {
				return true
			}
		}
	}
	
	return false
}

// isNumericIP quickly checks if a string looks like an IPv4 address without full parsing
func isNumericIP(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func main() {
	var quietMode bool
	var dryRun bool
	var trim bool
	flag.BoolVar(&quietMode, "q", false, "quiet mode (no output at all)")
	flag.BoolVar(&dryRun, "d", false, "don't write to file, just print the filtered result to stdout")
	flag.BoolVar(&trim, "t", false, "trim leading and trailing whitespace before comparison")
	flag.Parse()

	fn := flag.Arg(0)

	if fn == "" {
		fmt.Fprintf(os.Stderr, "error: no filename provided\n")
		return
	}

	// Read the target file lines into a slice to preserve order
	var fileLines []string
	r, err := os.Open(fn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open file for reading: %s\n", err)
		return
	}
	
	// Use a larger buffer for better I/O performance with large files
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024) // 64KB buffer
	scanner.Buffer(buf, 1024*1024)  // 1MB max token size
	
	for scanner.Scan() {
		fileLines = append(fileLines, scanner.Text())
	}
	r.Close()

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %s\n", err)
		return
	}

	// Read lines to remove from stdin, categorizing them by type
	exactMatches := make(map[string]bool)
	var wildcardPatterns []string
	var cidrRanges []string
	stdinScanner := bufio.NewScanner(os.Stdin)
	
	for stdinScanner.Scan() {
		line := stdinScanner.Text()
		if trim {
			line = strings.TrimSpace(line)
		}
		
		if strings.HasPrefix(line, "*.") {
			// Wildcard pattern for domains
			wildcardPatterns = append(wildcardPatterns, line)
		} else if strings.Contains(line, "/") {
			// Potential CIDR range
			if _, _, err := net.ParseCIDR(line); err == nil {
				cidrRanges = append(cidrRanges, line)
			} else {
				// Not a valid CIDR, treat as exact match
				exactMatches[line] = true
			}
		} else {
			// Exact match (domain, IP, or other string)
			exactMatches[line] = true
		}
	}

	// Pre-compile CIDR matchers for performance
	var cidrMatcher *CIDRMatcher
	if len(cidrRanges) > 0 {
		cidrMatcher = NewCIDRMatcher(cidrRanges)
	}

	// Filter the file lines, keeping only those not matching removal criteria
	var filteredLines []string
	for _, line := range fileLines {
		checkLine := line
		if trim {
			checkLine = strings.TrimSpace(line)
		}
		
		if !shouldRemoveLine(checkLine, exactMatches, wildcardPatterns, cidrMatcher) {
			filteredLines = append(filteredLines, line)
		}
	}

	// Output filtered lines to stdout if not in quiet mode
	if !quietMode {
		for _, line := range filteredLines {
			fmt.Println(line)
		}
	}

	// Write filtered lines back to file if not in dry-run mode
	if !dryRun {
		f, err := os.OpenFile(fn, os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to open file for writing: %s\n", err)
			return
		}
		defer f.Close()

		for _, line := range filteredLines {
			fmt.Fprintf(f, "%s\n", line)
		}
	}
}
