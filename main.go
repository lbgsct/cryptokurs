package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	// Флаги для выбора алгоритма и настроек
	algorithm := flag.String("algorithm", "loki97", "Choose encryption algorithm: loki97 or rc5")
	mode := flag.String("mode", "ECB", "Encryption mode: ECB, CBC, CFB, OFB, CTR, PCBC, RandomDelta")
	padding := flag.String("padding", "PKCS7", "Padding mode: Zeros, ANSIX923, PKCS7, ISO10126")
	key := flag.String("key", "1234567890abcdef", "Encryption key (16 bytes for LOKI97, 16/24/32 bytes for RC5)")
	iv := flag.String("iv", "", "Initialization vector (IV) in hex, required for CBC, CFB, OFB, CTR modes")
	input := flag.String("input", "input.txt", "Path to input file")
	output := flag.String("output", "output.txt", "Path to output file")
	decrypt := flag.Bool("decrypt", false, "Set to true for decryption")
	flag.Parse()

	// Проверка длины ключа
	if len(*key) != 16 && len(*key) != 24 && len(*key) != 32 {
		fmt.Println("Invalid key size. For LOKI97, use 16 bytes. For RC5, use 16, 24, or 32 bytes.")
		os.Exit(1)
	}

	// Парсим режим шифрования
	var cipherMode CipherMode
	switch *mode {
	case "ECB":
		cipherMode = ECB
	case "CBC":
		cipherMode = CBC
	case "CFB":
		cipherMode = CFB
	case "OFB":
		cipherMode = OFB
	case "CTR":
		cipherMode = CTR
	case "PCBC":
		cipherMode = PCBC
	case "RandomDelta":
		cipherMode = RandomDelta
	default:
		fmt.Println("Invalid encryption mode. Choose from: ECB, CBC, CFB, OFB, CTR, PCBC, RandomDelta")
		os.Exit(1)
	}

	// Парсим режим набивки
	var paddingMode PaddingMode
	switch *padding {
	case "Zeros":
		paddingMode = Zeros
	case "ANSIX923":
		paddingMode = ANSIX923
	case "PKCS7":
		paddingMode = PKCS7
	case "ISO10126":
		paddingMode = ISO10126
	default:
		fmt.Println("Invalid padding mode. Choose from: Zeros, ANSIX923, PKCS7, ISO10126")
		os.Exit(1)
	}

	// Парсим IV
	var ivBytes []byte
	if cipherMode != ECB && cipherMode != RandomDelta {
		if len(*iv) == 0 {
			fmt.Println("IV is required for selected encryption mode.")
			os.Exit(1)
		}
		ivBytes = make([]byte, 16)
		copy(ivBytes, *iv)
	}

	// Выбираем алгоритм
	var cipher SymmetricAlgorithm
	switch *algorithm {
	case "loki97":
		cipher = &Loki97{}
	case "rc5":
		//cipher = &RC5{} // Допустим, вы реализовали RC5 как другой SymmetricAlgorithm
	default:
		fmt.Println("Invalid algorithm. Choose 'loki97' or 'rc5'.")
		os.Exit(1)
	}

	// Устанавливаем ключ
	if err := cipher.SetKey([]byte(*key)); err != nil {
		fmt.Printf("Failed to set key: %v\n", err)
		os.Exit(1)
	}

	// Создаем контекст
	context, err := NewCryptoSymmetricContext(
		[]byte(*key),
		cipher,
		cipherMode,
		paddingMode,
		ivBytes,
		16, // Размер блока (например, 16 байт для LOKI97 и RC5)
	)
	if err != nil {
		fmt.Printf("Failed to create context: %v\n", err)
		os.Exit(1)
	}

	// Читаем входной файл
	inputData, err := os.ReadFile(*input)
	if err != nil {
		fmt.Printf("Failed to read input file: %v\n", err)
		os.Exit(1)
	}

	// Выполняем шифрование или дешифрование
	var outputData []byte
	if *decrypt {
		outputData, err = context.Decrypt(inputData)
		if err != nil {
			fmt.Printf("Decryption failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		outputData, err = context.Encrypt(inputData)
		if err != nil {
			fmt.Printf("Encryption failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Записываем результат в файл
	if err := os.WriteFile(*output, outputData, 0644); err != nil {
		fmt.Printf("Failed to write output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Operation completed successfully!")
}
