package pagetree

import (
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	rl := newRateLimiter()
	clock := time.Unix(1000, 0)
	rl.now = func() time.Time { return clock }

	for i := 0; i < defaultBurst; i++ {
		if !rl.allow("k1", "default") {
			t.Fatalf("запрос %d из burst отклонён", i+1)
		}
	}
	if rl.allow("k1", "default") {
		t.Error("burst исчерпан — запрос должен быть отклонён")
	}
	// Другой ключ не зависит
	if !rl.allow("k2", "default") {
		t.Error("второй ключ должен иметь свой бюджет")
	}
	// 100ms при 10 rps = ровно 1 токен
	clock = clock.Add(100 * time.Millisecond)
	if !rl.allow("k1", "default") {
		t.Error("после 100ms должен быть доступен 1 токен")
	}
	if rl.allow("k1", "default") {
		t.Error("после 100ms второго токена быть не должно")
	}
}

func TestRateLimiterUploadClass(t *testing.T) {
	rl := newRateLimiter()
	clock := time.Unix(1000, 0)
	rl.now = func() time.Time { return clock }

	if !rl.allow("k1", "upload") || !rl.allow("k1", "upload") {
		t.Fatal("burst upload = 2 должен пропускать 2 запроса")
	}
	if rl.allow("k1", "upload") {
		t.Error("третий upload должен быть отклонён")
	}
	// Обычный класс не тронут
	if !rl.allow("k1", "default") {
		t.Error("default-класс имеет отдельный бюджет")
	}
	// Через минуту токены восстановлены
	clock = clock.Add(time.Minute)
	if !rl.allow("k1", "upload") {
		t.Error("после минуты upload снова должен проходить")
	}
}
