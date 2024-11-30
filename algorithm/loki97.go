package algorithm

import (
	"encoding/binary"
	"errors"
)

// режим набивки и шифрования выбирает пользователь
const (
	numRounds = 16 // количество раундов шифрования в LOKI97 всегда 16
	blockSize = 16 // размер блока в байтах (128 бит)
	Delta     = 0x9e3779b9
)

// Определение интерфейсов
type Loki97 struct {
	key       []byte
	subKeys   [48][]byte
	blockSize int
}

//КЛЮЧИ

// Реализация интерфейса SymmetricAlgorithm
func (l *Loki97) SetKey(key []byte) error {
	if len(key) != 16 {
		return errors.New("key size must be 16 bytes")
	}
	Ka, Kb := KaKb(key)
	K1, K2, K3, K4 := generatefirstkeys(Ka, Kb)
	l.subKeys = generateSubkeys(K1, K2, K3, K4)
	l.key = key
	l.blockSize = 16 // Блоки 128 бит
	return nil
}

// делим на два ключа по 64 бита(8 байт)
func KaKb(Key []byte) ([]byte, []byte) {

	size := len(Key) / 2
	Ka := Key[:size]
	Kb := Key[size:]

	return Ka, Kb
}

// генерация первых 4 ключей
func generatefirstkeys(Ka, Kb []byte) ([]byte, []byte, []byte, []byte) {
	// Создаем копии Ka и Kb для K4 и K3
	K4 := make([]byte, len(Ka))
	copy(K4, Ka)

	K3 := make([]byte, len(Kb))
	copy(K3, Kb)

	K2 := f(makeCopy(Kb), makeCopy(Ka))
	K1 := f(makeCopy(Ka), makeCopy(Kb))

	return K1, K2, K3, K4
}

// генерация подключей для 16 раундов сети Фейстеля
func generateSubkeys(K1, K2, K3, K4 []byte) [48][]byte {

	var SKeys [48][]byte

	// 48 раундов
	for i := 0; i < 48; i++ {
		SKeys[i] = XOR(K1, g(K1, K3, K2, i))

		// Обновляем значения ключей для следующего раунда
		K1, K2, K3, K4 = K4, K1, K2, K3
	}

	return SKeys
}

// доп функция для генерации подключей
func g(K1, K3, K2 []byte, i int) []byte {

	deltaMultiplied := Delta * uint64(i)
	Firts := Plus(Plus(K1, K3), uint64ToBytes(deltaMultiplied))

	return f(Firts, K2)
}

//шифровка  и дешифровка

func (l *Loki97) Encrypt(data []byte) ([]byte, error) {
	if len(data) != l.blockSize {
		return nil, errors.New("data size must be 16 bytes")
	}

	L0, R0 := LRBlocks(data)
	encrypted := FeistelNetwork(L0, R0, l.subKeys)
	return encrypted, nil
}

func (l *Loki97) Decrypt(data []byte) ([]byte, error) {
	if len(data) != l.blockSize {
		return nil, errors.New("data size must be exactly 16 bytes")
	}

	// Разделяем блок на левую и правую части
	L0, R0 := LRBlocks(data)

	// Выполняем дешифрование через сеть Фейстеля
	decryptedBlock := decr(L0, R0, l.subKeys)

	return decryptedBlock, nil
}

func decr(L, R []byte, SKeys [48][]byte) []byte {
	// 16 раундов дешифрования
	for i := 16; i >= 1; i-- {
		// Получаем подключи для текущего раунда
		SK3 := SKeys[(3*i)%48]   // SK_(3i)
		SK2 := SKeys[(3*i-1)%48] // SK_(3i-2)
		SK1 := SKeys[(3*i-2)%48] // SK_(3i-1)

		// Обновляем значения R и L
		newR := XOR(L, f(Minus(R, SK3), SK2))
		newL := Minus(Minus(R, SK3), SK1)

		// Обновляем L и R для следующего раунда
		R = newR
		L = newL
	}

	// Объединяем результат в один блок (128 бит или 16 байт)
	result := make([]byte, 16)
	copy(result[:8], R) // R становится левой частью
	copy(result[8:], L) // L становится правой частью

	return result
}

//S блоки

// Sa использует S1 S2
func Sa(A []byte) []byte {
	result := make([]byte, 8)

	// Чередуем S1 и S2 в соответствии с заданной последовательностью
	for i := 0; i < 8; i++ {
		if i%2 == 0 {
			result[i] = S1(uint16(A[i]))
		} else {
			result[i] = S2(uint16(A[i]))
		}
	}

	return result
}

// S1 и S2 — это S-боксы
func Sb(A []byte, B []byte) []byte {
	result := make([]byte, 8) // Результат будет 64 бита

	// Применяем S2 и S1 по описанному порядку для Sb
	for i := 0; i < 8; i++ {
		if i%2 == 0 {
			result[i] = S2(uint16(A[i])) // Применяем S2 к данным из Sa()
		} else {
			result[i] = S1(uint16(B[i])) // Применяем S1 к ключевым данным
		}
	}

	return result
}

// умножение по модулю многочлена для поля Галуа вынести в отдельный файлик
func galoisMultiply(a, b uint16, mod uint16, bits int) uint16 {
	var result uint16 = 0
	var ax uint16 = a //для того чтобы не ругался на размер

	for i := 0; i < bits; i++ {
		if (b & 1) != 0 {
			result ^= ax
		}
		ax <<= 1
		// проверка старшего бита, чтобы выполнить модульное сокращение
		if (ax & (1 << bits)) != 0 {
			ax ^= mod
		}
		b >>= 1
	}
	return result
}

// возводим в куб в поле Галуа GF(2^n) с порождающим полиномом
func galoisCube(x uint16, mod uint16, bits int) uint16 {
	// в квадрат
	x2 := galoisMultiply(x, x, mod, bits)

	// куб (умножаем x2 на x)
	x3 := galoisMultiply(x2, x, mod, bits)

	return x3
}

// S1 кубирование в GF(2^13), инверсия и маскирование
func S1(x uint16) byte {

	x = ^x & 0x1FFF // инвертируем и ограничиваем 13 битами

	// кубируем в поле GF(2^13) с порождающим полиномом 0x2911
	xCubed := galoisCube(x, 0x2911, 13)

	// маскирование, чтобы получить только 8 младших бит
	return byte(xCubed & 0xFF)
}

// S2 кубирование в GF(2^11), инверсия и маскирование
func S2(x uint16) byte {

	x = ^x & 0x7FF // инвертируем и ограничиваем 11 битами

	// кубируем в поле GF(2^11) с порождающим полиномом 0xA7
	xCubed := galoisCube(x, 0xA7, 11)

	// маскирование, чтобы получить только 8 младших бит
	return byte(xCubed & 0xFF)
}

// разделяем 128 блок на левую и правую часть по 64 бита
func LRBlocks(block []byte) ([]byte, []byte) {

	size := len(block) / 2
	L := block[:size]
	R := block[size:]

	return L, R
}

// функция шифрования
func f(A, B []byte) []byte {
	A = Sb(P(Sa(E(KP(A, B)))), B)
	return A
}

// ключевая перестановка
func KP(A, B []byte) []byte {
	Al, Ar := KaKb(A) //делим на 32 бита
	_, SKr := KaKb(B) //берем младшие 32 бита

	// проходим по 32 битам
	for i := 0; i < len(SKr)*8; i++ {
		// извлекаем бит
		bit := (SKr[i/8] >> (7 - i%8)) & 1

		// Если бит в SKr равен 1, меняем местами соответствующие биты Al и Ar
		if bit == 1 {
			// Определяем индекс бита в байте
			byteIndex := i / 8
			bitIndex := i % 8

			// Получаем биты из Al и Ar
			AlBit := (Al[byteIndex] >> (7 - bitIndex)) & 1
			ArBit := (Ar[byteIndex] >> (7 - bitIndex)) & 1

			// Если биты разные, меняем их местами
			if AlBit != ArBit {
				// Переставляем биты на нужных местах в Al и Ar
				Al[byteIndex] ^= (1 << (7 - bitIndex))
				Ar[byteIndex] ^= (1 << (7 - bitIndex))
			}
		}
	}
	result := append(Al, Ar...) // результат должен быть длиной 8 байт (64 бита)
	return result
}

// функция расширения для 64 битов
func E(input []byte) []byte {
	// Проверка на корректность длины входных данных (должны быть 8 байтов = 64 бита)
	if len(input) != 8 {
		panic("Input must be 64 bits (8 bytes)")
	}

	// Создаем массив для 96 бит (12 байтов)
	result := make([]byte, 12)

	// Перестановка, где каждый элемент в `result` будет заполняться соответствующими битами из `input`
	result[0] = (input[0] >> 4) & 0x1F  // биты 4:0
	result[1] = (input[7] >> 6) & 0x3F  // биты 63:56
	result[2] = (input[5] >> 4) & 0xFF  // биты 58:48
	result[3] = (input[4] >> 4) & 0xFF  // биты 52:40
	result[4] = (input[3] >> 3) & 0xFF  // биты 42:32
	result[5] = (input[2] >> 2) & 0xFF  // биты 34:24
	result[6] = (input[1] >> 2) & 0xFF  // биты 28:16
	result[7] = (input[7] >> 0) & 0xFF  // биты 18:8
	result[8] = (input[6] >> 0) & 0xFF  // биты 12:0
	result[9] = (input[5] >> 5) & 0x1F  // биты 63:56
	result[10] = (input[4] >> 3) & 0xFF // биты 52:40
	result[11] = (input[3] >> 6) & 0xFF // биты 42:32

	return result
}

// перестановка из s box в более
func P(input []byte) []byte {
	// Позиции для перестановки, начиная с 1 (для удобства)   один раз объявить в константу
	perm := []int{
		56, 48, 40, 32, 24, 16, 8, 0,
		57, 49, 41, 33, 25, 17, 9, 1,
		58, 50, 42, 34, 26, 18, 10, 2,
		59, 51, 43, 35, 27, 19, 11, 3,
		60, 52, 44, 36, 28, 20, 12, 4,
		61, 53, 45, 37, 29, 21, 13, 5,
		62, 54, 46, 38, 30, 22, 14, 6,
		63, 55, 47, 39, 31, 23, 15, 7,
	}

	output := make([]byte, 8) // Результат в 64 бита (8 байт)

	for i, p := range perm {
		// Получаем бит на нужной позиции
		bit := (input[p/8] >> (7 - p%8)) & 1

		// Устанавливаем этот бит в новую позицию на выходе
		if bit == 1 {
			output[i/8] |= (1 << (7 - i%8))
		}
	}

	return output
}

//Доп функции

func XOR(a, b []byte) []byte {
	if len(a) != len(b) {
		panic("Slices must have the same length")
	}
	result := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = a[i] ^ b[i]
	}
	return result
}

func Plus(a, b []byte) []byte {
	if len(a) != len(b) {
		panic("Slices must have the same length")
	}

	result := make([]byte, len(a))
	carry := uint64(0)

	for i := len(a) - 1; i >= 0; i-- {
		sum := uint64(a[i]) + uint64(b[i]) + carry
		result[i] = byte(sum & 0xFF)
		carry = sum >> 8
	}

	return result
}

// Функция для вычитания байтовых массивов
func Minus(a, b []byte) []byte {
	if len(a) != len(b) {
		panic("Slices must have the same length")
	}

	result := make([]byte, len(a))
	borrow := uint64(0)

	for i := len(a) - 1; i >= 0; i-- {
		diff := int64(a[i]) - int64(b[i]) - int64(borrow)
		if diff < 0 {
			result[i] = byte((diff + 256) & 0xFF)
			borrow = 1
		} else {
			result[i] = byte(diff & 0xFF)
			borrow = 0
		}
	}

	return result
}

func uint32ToBytes(n uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, n)
	return b
}

func makeCopy(src []byte) []byte {
	copis := make([]byte, len(src))
	copy(copis, src)
	return copis
}

func uint64ToBytes(n uint64) []byte {

	result := make([]byte, 8)

	for i := 0; i < 8; i++ {
		result[i] = byte(n >> (56 - 8*i))
	}

	return result
}

func bytesToUint64(b []byte) uint64 {

	if len(b) != 8 {
		panic("Expected 8 bytes")
	}

	var result uint64

	for i := 0; i < 8; i++ {
		result |= uint64(b[i]) << (56 - 8*i)
	}

	return result
}

func FeistelNetwork(L, R []byte, SKeys [48][]byte) []byte {
	for i := 1; i <= 16; i++ {

		SK3 := SKeys[(3*i)%48]   // SK_(3i)
		SK2 := SKeys[(3*i-1)%48] // SK_(3i-2)
		SK1 := SKeys[(3*i-2)%48] // SK_(3i-1)

		// Обновление правой и левой частей
		newR := XOR(L, f(Plus(R, SK1), SK2)) // Обновление правой части
		newL := Plus(Plus(R, SK1), SK3)      // Обновление левой части

		// Обновление L и R для следующего раунда
		L = newL
		R = newR

	}

	// Объединяем результат
	result := make([]byte, 16)
	copy(result[:8], R)
	copy(result[8:], L)

	return result
}
