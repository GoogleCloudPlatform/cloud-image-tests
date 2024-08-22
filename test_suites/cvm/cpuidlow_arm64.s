//go:build arm64

#include "textflag.h"

// func cpuidlow(leaf, subleaf uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuidlow(SB), NOSPLIT, $0-24
    // Simply return zero values
    MOVD $0, R0  // eax
    MOVD $0, R1  // ebx
    MOVD $0, R2  // ecx
    MOVD $0, R3  // edx
    RET
