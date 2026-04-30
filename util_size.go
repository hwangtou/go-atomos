package atomos

import (
	"strconv"
	"strings"
)

// UtilFileSizeToString convert size to string.
// The size will be converted to B, KB, MB, GB or TB.
func UtilFileSizeToString(size int64, precision int) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + "B"
	}
	if size < 1024*1024 {
		return strconv.FormatFloat(float64(size)/1024, 'f', precision, 64) + "KB"
	}
	if size < 1024*1024*1024 {
		return strconv.FormatFloat(float64(size)/1024/1024, 'f', precision, 64) + "MB"
	}
	if size < 1024*1024*1024*1024 {
		return strconv.FormatFloat(float64(size)/1024/1024/1024, 'f', precision, 64) + "GB"
	}
	return strconv.FormatFloat(float64(size)/1024/1024/1024/1024, 'f', precision, 64) + "TB"
}

// UtilStringToFileSize convert string to disk size.
// The string should be ended with B, KB, MB, GB or TB.
func UtilStringToFileSize(sizeStr string) (int64, *Error) {
	if sizeStr == "" {
		return 0, NewError(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: The string should not be empty.")
	}
	if strings.HasSuffix(sizeStr, "B") {
		if len(sizeStr) < 2 {
			return 0, NewErrorf(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: The string should be ended with B, KB, MB, GB or TB. size=(%s)", sizeStr)
		}
		unit := sizeStr[len(sizeStr)-2:]
		valueStr := ""
		times := 0
		switch unit {
		case "KB":
			valueStr = sizeStr[:len(sizeStr)-2]
			times = 1024
		case "MB":
			valueStr = sizeStr[:len(sizeStr)-2]
			times = 1024 * 1024
		case "GB":
			valueStr = sizeStr[:len(sizeStr)-2]
			times = 1024 * 1024 * 1024
		case "TB":
			valueStr = sizeStr[:len(sizeStr)-2]
			times = 1024 * 1024 * 1024 * 1024
		default:
			valueStr = sizeStr[:len(sizeStr)-1]
			times = 1
		}
		if strings.Contains(valueStr, ".") {
			v, er := strconv.ParseFloat(valueStr, 64)
			if er != nil {
				return 0, NewErrorf(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: Invalid size string. size=(%s)", sizeStr)
			}
			return int64(v * float64(times)), nil
		} else {
			v, er := strconv.ParseInt(valueStr, 10, 64)
			if er != nil {
				return 0, NewErrorf(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: Invalid size string. size=(%s)", sizeStr)
			}
			return v * int64(times), nil
		}
	} else {
		v, er := strconv.ParseInt(sizeStr, 10, 64)
		if er != nil {
			return 0, NewErrorf(ErrFrameworkIncorrectUsage, "UtilStringToFileSize: The string should be ended with B, KB, MB, GB or TB. size=(%s)", sizeStr)
		}
		return v, nil
	}
}
