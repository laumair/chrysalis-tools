package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	mathrand "math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/iotaledger/iota.go/address"
	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/bundle"
	"github.com/iotaledger/iota.go/consts"
	"github.com/iotaledger/iota.go/kerl"
	"github.com/iotaledger/iota.go/kerl/sha3"

	"github.com/iotaledger/iota.go/pow"
	"github.com/iotaledger/iota.go/transaction"
	"github.com/iotaledger/iota.go/trinary"
	"github.com/wollac/iota-crypto-demo/pkg/bip32path"
	"github.com/wollac/iota-crypto-demo/pkg/bip39"
	"github.com/wollac/iota-crypto-demo/pkg/slip10"

	"golang.org/x/text/unicode/norm"
)

var (
	nodeAPIURI               = flag.String("node", "https://nodes-legacy.ledgermigration1.net", "the API URI of the node")
	originSeed               = flag.String("seed", strings.Repeat("9", consts.HashTrytesSize), "the seed to use to fund the created bundles")
	infoFileName             = flag.String("info-file", "bundles.csv", "the file containing the different generated bundles")
	manyAddrsCount           = flag.Int("manyAddrsCount", 15, "the addrs count to use for scenarios which involve many addresses")
	manyAddrsSpace           = flag.Int("manyAddrsSpace", 15, "the index space to use for scenarios which involve many addresses")
	manyAddrsSpentCount      = flag.Int("manyAddrsSpentCount", 15, "the addrs count to use for scenarios which involve many spent addresses")
	manyAddrsSpentSpace      = flag.Int("manyAddrsSpentSpace", 15, "the index space to use for scenarios which involve many spent addresses")
	manyAddrsSpentMixedCount = flag.Int("manyAddrsSpentMixedCount", 15, "the addrs count to use for scenarios which involve many unspent/spent addresses")
	manyAddrsSpentMixedSpace = flag.Int("manyAddrsSpentMixedSpace", 15, "the index space to use for scenarios which involve many unspent/spent addresses")
	mwm                      = flag.Int("mwm", 14, "the mwm to use for generated transactions/bundles")
)

const UPPER_INDEX = 3
const ACCOUNT_INDEX = 508396330

var seeds_count = 0

const BIP_BASE_PATH = "44'/4218'/%d'/0'"

var MNEMONIC bip39.Mnemonic

func init() {
	mathrand.Seed(time.Now().Unix())
}

func must(args ...interface{}) {
	for _, arg := range args {
		if arg == nil {
			continue
		}
		if _, ok := arg.(error); !ok {
			continue
		}
		panic(arg)
	}
}

func main() {
	flag.Parse()

	_, powF := pow.GetFastestProofOfWorkImpl()
	legacyAPI, err := api.ComposeAPI(api.HTTPClientSettings{
		URI:                  *nodeAPIURI,
		LocalProofOfWorkFunc: powF,
	})
	must(err)

	originAddr := mustAddrWithChecksum(*originSeed, 0)

	balancesRes, err := legacyAPI.GetBalances(trinary.Hashes{originAddr})
	must(err)

	log.Printf("there are %d tokens residing on the first address of the specified seed", balancesRes.Balances[0])

	generateBundles(legacyAPI, originAddr)
}

func generateBundles(legacyAPI *api.API, originAddr trinary.Trytes) {
	s := time.Now()
	if err := os.Remove(*infoFileName); err != nil && !os.IsNotExist(err) {
		panic(err)
	}

	infoFile, err := os.OpenFile(*infoFileName, os.O_RDWR|os.O_CREATE, 0666)
	must(err)
	defer infoFile.Close()

	scenario(
		fmt.Sprintf("Funds (>=1Mi) on a single unspent address (low index; < %d)", UPPER_INDEX),
		fmt.Sprintf("Test migration with a seed with all funds on a single unspent address with index < %d.", UPPER_INDEX),
		1_500_000, func() []AddrTuple {
			targetSeed, targetMnemonic := randSeed()
			targetAddrIndex := uint64(mathrand.Int63n(UPPER_INDEX))

			return []AddrTuple{
				{
					Seed:     targetSeed,
					Mnemonic: targetMnemonic,
					Index:    targetAddrIndex,
					Addr:     mustAddrWithChecksum(targetSeed, targetAddrIndex),
					Value:    1_500_000,
					Spent:    false,
				},
			}
		}(), legacyAPI, []api.Input{
			{
				Balance:  1_500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario(
		fmt.Sprintf("Funds (<1Mi) on a single unspent address (low index; < %d)", UPPER_INDEX),
		fmt.Sprintf("Test migration with a seed with all funds on a single unspent address with index < %d.", UPPER_INDEX),
		500_000, func() []AddrTuple {
			targetSeed, targetMnemonic := randSeed()
			targetAddrIndex := uint64(mathrand.Int63n(UPPER_INDEX))

			return []AddrTuple{
				{
					Seed:     targetSeed,
					Mnemonic: targetMnemonic,
					Index:    targetAddrIndex,
					Addr:     mustAddrWithChecksum(targetSeed, targetAddrIndex),
					Value:    500_000,
					Spent:    false,
				},
			}
		}(), legacyAPI, []api.Input{
			{
				Balance:  500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds (>=1Mi) on a single unspent address (high index)",
		fmt.Sprintf("Test migration with a seed with all funds on a single unspent address with index > %d.", UPPER_INDEX),
		1_500_000, func() []AddrTuple {
			targetSeed, targetMnemonic := randSeed()
			targetAddrIndex := uint64(mathrand.Int63n(10) + UPPER_INDEX + 1)

			return []AddrTuple{
				{
					Seed:     targetSeed,
					Mnemonic: targetMnemonic,
					Index:    targetAddrIndex,
					Addr:     mustAddrWithChecksum(targetSeed, targetAddrIndex),
					Value:    1_500_000,
					Spent:    false,
				},
			}
		}(), legacyAPI, []api.Input{
			{
				Balance:  1_500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds (<1Mi) spread across many addresses",
		fmt.Sprintf("Test migration with a seed with funds < 1Mi spread across at least %d addresses unevenly (not in sequence but rather across random address indexes)", *manyAddrsCount),
		500_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsCount, *manyAddrsSpace, betweenEvenSpread(500_000, *manyAddrsCount), 0)
		}(), legacyAPI, []api.Input{
			{
				Balance:  500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds (>=1Mi) spread across many addresses",
		fmt.Sprintf("Test migration with a seed with funds >=1Mi spread across at least %d addresses unevenly (not in sequence but rather across random address indexes)", *manyAddrsCount),
		5_000_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsCount, *manyAddrsSpace, betweenEvenSpread(5_000_000, *manyAddrsCount), 0)
		}(), legacyAPI, []api.Input{
			{
				Balance:  5_000_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds spread across many addresses with each >=1Mi",
		fmt.Sprintf("Test migration with a seed with funds spread across at least %d addresses with each having >=1Mi unevenly (not in sequence but rather across random address indexes)", *manyAddrsCount),
		uint64(*manyAddrsCount)*1_500_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsCount, *manyAddrsSpace, func(index uint64) uint64 {
				// all get the same amount
				return 1_500_000
			}, 0)
		}(), legacyAPI, []api.Input{
			{
				Balance:  uint64(*manyAddrsCount) * 1_500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Mixture of funds (>=1Mi & <1Mi) spread across many unspent addresses",
		fmt.Sprintf("Test migration with a seed with a mixture of funds >=1Mi & <1Mi spread across at least %d unspent addresses unevenly", *manyAddrsCount),
		50_000_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsCount, *manyAddrsSpace, betweenMaxAOrB(50_000_000, *manyAddrsCount, 1000, 1_000_000, 0.10), 0)
		}(), legacyAPI, []api.Input{
			{
				Balance:  50_000_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario(
		fmt.Sprintf("Funds (>=1Mi) on a single spent address (low index; < %d)", UPPER_INDEX),
		fmt.Sprintf("Test migration with a seed with all funds >=1Mi on a single spent address with index < %d.", UPPER_INDEX),
		1_500_000, func() []AddrTuple {
			targetSeed, targetMnemonic := randSeed()
			targetAddrIndex := uint64(mathrand.Int63n(UPPER_INDEX))

			return []AddrTuple{
				{
					Seed:     targetSeed,
					Mnemonic: targetMnemonic,
					Index:    targetAddrIndex,
					Addr:     mustAddrWithChecksum(targetSeed, targetAddrIndex),
					Value:    1_500_000,
					Spent:    true,
				},
			}
		}(), legacyAPI, []api.Input{
			{
				Balance:  1_500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario(
		fmt.Sprintf("Funds (<1Mi) on a single spent address (low index; < %d)", UPPER_INDEX),
		fmt.Sprintf("Test migration with a seed with all funds on a single spent address with index < %d.", UPPER_INDEX),
		500_000, func() []AddrTuple {
			targetSeed, targetMnemonic := randSeed()
			targetAddrIndex := uint64(mathrand.Int63n(UPPER_INDEX))

			return []AddrTuple{
				{
					Seed:     targetSeed,
					Mnemonic: targetMnemonic,
					Index:    targetAddrIndex,
					Addr:     mustAddrWithChecksum(targetSeed, targetAddrIndex),
					Value:    500_000,
					Spent:    true,
				},
			}
		}(), legacyAPI, []api.Input{
			{
				Balance:  500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds (<1Mi) spread across many spent addresses",
		fmt.Sprintf("Test migration with a seed with funds < 1Mi spread across at least %d spent addresses unevenly (not in sequence but rather across random address indexes)", *manyAddrsSpentCount),
		500_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsSpentCount, *manyAddrsSpentSpace, betweenEvenSpread(500_000, *manyAddrsSpentCount), 1.0)
		}(), legacyAPI, []api.Input{
			{
				Balance:  500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds (>=1Mi) spread across many spent addresses",
		fmt.Sprintf("Test migration with a seed with funds >=1Mi spread across at least %d spent addresses unevenly (not in sequence but rather across random address indexes)", *manyAddrsSpentCount),
		5_000_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsSpentCount, *manyAddrsSpentSpace, betweenEvenSpread(5_000_000, *manyAddrsSpentCount), 1)
		}(), legacyAPI, []api.Input{
			{
				Balance:  5_000_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds spread across many spent addresses with each >=1Mi",
		fmt.Sprintf("Test migration with a seed with funds spread across at least %d spent addresses with each having >=1Mi unevenly (not in sequence but rather across random address indexes)", *manyAddrsSpentCount),
		uint64(*manyAddrsSpentCount)*1_500_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsSpentCount, *manyAddrsSpentSpace, func(index uint64) uint64 {
				// all get the same amount
				return 1_500_000
			}, 1)
		}(), legacyAPI, []api.Input{
			{
				Balance:  uint64(*manyAddrsSpentCount) * 1_500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Mixture of funds (>=1Mi & <1Mi) spread across many spent addresses",
		fmt.Sprintf("Test migration with a seed with a mixture of funds >=1Mi & <1Mi spread across at least %d spent addresses unevenly", *manyAddrsSpentCount),
		50_000_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsSpentCount, *manyAddrsSpentSpace, betweenMaxAOrB(50_000_000, *manyAddrsSpentCount, 1000, 1_000_000, 0.10), 1)
		}(), legacyAPI, []api.Input{
			{
				Balance:  50_000_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Mixture of funds (>=1Mi & <1Mi) spread across both spent and unspent addresses",
		fmt.Sprintf("Test migration with a seed with a mixture of funds >=1Mi & <1Mi spread across at least %d spent and unspent addresses", *manyAddrsSpentMixedCount),
		50_000_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsSpentMixedCount, *manyAddrsSpentMixedSpace, betweenMaxAOrB(50_000_000, *manyAddrsSpentMixedCount, 1000, 1_000_000, 0.10), 0.25)
		}(), legacyAPI, []api.Input{
			{
				Balance:  50_000_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	scenario("Funds spread across many spent and unspent addresses with each >=1Mi",
		fmt.Sprintf("Test migration with a seed with funds spread across at least %d spent and unspent addresses with each having >=1Mi unevenly (not in sequence but rather across random address indexes)", *manyAddrsSpentMixedCount),
		uint64(*manyAddrsSpentMixedCount)*1_500_000, func() []AddrTuple {
			return fundsSpreadAcrossAddrSpace(*manyAddrsSpentMixedCount, *manyAddrsSpentMixedSpace, func(index uint64) uint64 {
				// all get the same amount
				return 1_500_000
			}, 0.25)
		}(), legacyAPI, []api.Input{
			{
				Balance:  uint64(*manyAddrsSpentMixedCount) * 1_500_000,
				Address:  originAddr,
				KeyIndex: 0,
				Security: consts.SecurityLevelMedium,
			},
		}, infoFile)

	log.Printf("done, goodbye! %v\n", time.Since(s))
}

type FundsOnAddr func(index uint64) uint64

func betweenEvenSpread(funds uint64, addrCount int) FundsOnAddr {
	evenSpread := funds / uint64(addrCount)
	var sum uint64
	var calls int
	return func(_ uint64) uint64 {
		if calls+1 == addrCount {
			return funds - sum
		}
		fundsForAddr := uint64(mathrand.Int63n(int64(evenSpread)) + 1)
		sum += fundsForAddr
		calls++
		return fundsForAddr
	}
}

func betweenMaxAOrB(funds uint64, addrCount int, a uint64, b uint64, chanceB float64) FundsOnAddr {
	var sum uint64
	var calls int
	return func(_ uint64) uint64 {
		if calls+1 == addrCount {
			return funds - sum
		}

		fundsForAddr := uint64(mathrand.Int63n(int64(a)) + 1)
		if chanceB > mathrand.Float64() {
			fundsForAddr = b
		}

		sum += fundsForAddr
		calls++
		return fundsForAddr
	}
}

func fundsSpreadAcrossAddrSpace(addrCount int, addrSpace int, fundsOnAddr FundsOnAddr, chanceOfSpent float64) []AddrTuple {
	targetSeed, targetMnemonic := randSeed()

	targetAddrs := make([]AddrTuple, 0)
	used := make(map[uint64]struct{})
	for i := 0; i < addrCount; i++ {
		var addrIndex uint64
		for {
			addrIndex = uint64(mathrand.Intn(addrSpace))
			if _, ok := used[addrIndex]; !ok {
				break
			}
		}
		used[addrIndex] = struct{}{}

		var spent bool
		if chanceOfSpent != 0 && chanceOfSpent > mathrand.Float64() {
			spent = true
		}

		targetAddrs = append(targetAddrs, AddrTuple{
			Seed:     targetSeed,
			Mnemonic: targetMnemonic,
			Index:    addrIndex,
			Addr:     mustAddrWithChecksum(targetSeed, addrIndex),
			Value:    fundsOnAddr(addrIndex),
			Spent:    spent,
		})
	}

	sort.Slice(targetAddrs, func(i, j int) bool {
		return targetAddrs[i].Index < targetAddrs[j].Index
	})

	return targetAddrs
}

func waitUntilConfirmed(legacyAPI *api.API, tailTx *transaction.Transaction) {
	for {
		inclState, err := legacyAPI.GetInclusionStates(trinary.Hashes{tailTx.Hash})
		must(err)

		if inclState[0] {
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func sendPrepBundle(legacyAPI *api.API, infoFile io.Writer, prepBundle []trinary.Trytes) *transaction.Transaction {
	tipsRes, err := legacyAPI.GetTransactionsToApprove(3)
	must(err)

	rdyBundle, err := legacyAPI.AttachToTangle(tipsRes.TrunkTransaction, tipsRes.BranchTransaction, uint64(*mwm), prepBundle)
	must(err)

	tailTx, err := transaction.AsTransactionObject(rdyBundle[0])
	must(err)

	must(fmt.Fprintf(infoFile, "tail tx: %s\nbundle hash: %s\n", tailTx.Hash, tailTx.Bundle))

	must(legacyAPI.BroadcastTransactions(rdyBundle...))

	return tailTx
}

func makeAddrsSpent(legacyAPI *api.API, infoFile io.Writer, addrsTuple []AddrTuple) {
	burnerSeed, _ := randBurnerSeed()

	var transfers, backTransfers = make(bundle.Transfers, 0), make(bundle.Transfers, 0)
	var inputs, backInputs = make([]api.Input, 0), make([]api.Input, 0)

	for _, addrTuple := range addrsTuple {
		if !addrTuple.Spent {
			continue
		}

		burnerAddr, err := address.GenerateAddress(burnerSeed, addrTuple.Index, consts.SecurityLevelMedium, true)
		must(err)

		transfers = append(transfers, bundle.Transfer{Address: burnerAddr, Value: addrTuple.Value})
		backTransfers = append(backTransfers, bundle.Transfer{
			Address: addrTuple.Addr,
			Value:   addrTuple.Value,
		})

		inputs = append(inputs, api.Input{
			Balance:  addrTuple.Value,
			Address:  addrTuple.Addr,
			KeyIndex: addrTuple.Index,
			Security: consts.SecurityLevelMedium,
		})
		backInputs = append(backInputs, api.Input{
			Balance:  addrTuple.Value,
			Address:  burnerAddr,
			KeyIndex: addrTuple.Index,
			Security: consts.SecurityLevelMedium,
		})
	}

	sendOffOpts := api.PrepareTransfersOptions{Inputs: inputs, Security: consts.SecurityLevelMedium}
	sendOff, err := legacyAPI.PrepareTransfers(addrsTuple[0].Seed, transfers, sendOffOpts)
	must(err)

	tailTx := sendPrepBundle(legacyAPI, ioutil.Discard, sendOff)
	log.Printf("waiting for sent-off bundle to be confirmed then sending back... (tail %s)", tailTx.Hash)
	waitUntilConfirmed(legacyAPI, tailTx)
	log.Println("sent-off bundle confirmed, sending back to spent addresses...")

	sendBackOpts := api.PrepareTransfersOptions{Inputs: backInputs, Security: consts.SecurityLevelMedium}
	sendBack, err := legacyAPI.PrepareTransfers(burnerSeed, backTransfers, sendBackOpts)
	must(err)

	sendPrepBundle(legacyAPI, infoFile, sendBack)
}

type AddrTuple struct {
	Seed     trinary.Trytes
	Mnemonic bip39.Mnemonic
	Index    uint64
	Addr     trinary.Hash
	Value    uint64
	Spent    bool
}

func mustAddrWithChecksum(seed string, index uint64) trinary.Trytes {
	targetAddr, err := address.GenerateAddress(seed, index, consts.SecurityLevelMedium, true)
	must(err)
	return targetAddr
}

func checkInputSeeds(addrsTuple []AddrTuple, infoFile *os.File) bool {
	uniqueSeeds := make(map[string]struct{})
	for _, addrTuple := range addrsTuple {
		uniqueSeeds[addrTuple.Seed] = struct{}{}
	}

	printSeedPerAddr := true
	if len(uniqueSeeds) == 1 {
		printSeedPerAddr = false
		for k := range uniqueSeeds {
			must(fmt.Fprintf(infoFile, "seed %s\n", k))
			break
		}
	}

	return printSeedPerAddr
}

func scenario(name string, desc string, funds uint64, addrsTuple []AddrTuple, legacyAPI *api.API, inputs []api.Input, infoFile *os.File) {
	log.Printf("generating scenario: %s\n", name)
	log.Printf("description: %s\n", desc)
	s := time.Now()
	must(fmt.Fprintf(infoFile, "scenario: %s\n", name))
	must(fmt.Fprintf(infoFile, "description: %s\n", desc))
	must(fmt.Fprintf(infoFile, "account balance: %d\n", funds))

	must(fmt.Fprintf(infoFile, "mnemonic: %s\n", addrsTuple[0].Mnemonic))
	must(fmt.Fprintf(infoFile, "BIP path: %s\n", fmt.Sprintf(BIP_BASE_PATH, ACCOUNT_INDEX+seeds_count-1)))
	must(fmt.Fprintf(infoFile, "Account Index: %d\n", seeds_count-1))

	defer func() {
		log.Printf("done generating scenario, took %v\n", time.Since(s))
		must(fmt.Fprintf(infoFile, "took %v\n", time.Since(s)))
		must(fmt.Fprintf(infoFile, "\n#####################################\n"))
	}()

	printSeedPerAddr := checkInputSeeds(addrsTuple, infoFile)

	transfers := bundle.Transfers{}
	var shouldAnyBeSpent bool
	for _, addrTuple := range addrsTuple {
		transfers = append(transfers, bundle.Transfer{Address: addrTuple.Addr, Value: addrTuple.Value})
		if addrTuple.Spent {
			shouldAnyBeSpent = true
		}

		if printSeedPerAddr {
			must(fmt.Fprintf(infoFile, "seed %s\naddr index %d: %s, spent=%v, - %d\n", addrTuple.Seed, addrTuple.Index, addrTuple.Addr, addrTuple.Spent, addrTuple.Value))
			continue
		}
		must(fmt.Fprintf(infoFile, "addr index %d: %s, spent=%v - %d\n", addrTuple.Index, addrTuple.Addr, addrTuple.Spent, addrTuple.Value))
	}

	opts := api.PrepareTransfersOptions{Inputs: inputs, Security: consts.SecurityLevelMedium}
	prepBundle, err := legacyAPI.PrepareTransfers(*originSeed, transfers, opts)
	must(err)

	tailTx := sendPrepBundle(legacyAPI, infoFile, prepBundle)

	if !shouldAnyBeSpent {
		return
	}

	log.Printf("waiting for scenario bundle to be confirmed before sending forth/back to spent addrs... (tail %s)", tailTx.Hash)
	waitUntilConfirmed(legacyAPI, tailTx)
	makeAddrsSpent(legacyAPI, infoFile, addrsTuple)
}

func randBurnerSeed() (string, bip39.Mnemonic) {
	var (
		err      error
		entropy  []byte
		mnemonic bip39.Mnemonic
	)

	if err != nil {
		fmt.Errorf("failed generating entropy: %w", err)
	}

	entropy, err = generateEntropy(256 / 8)

	mnemonic, _ = bip39.EntropyToMnemonic(entropy)

	seed, _ := bip39.MnemonicToSeed(mnemonic, "")

	var bip_path = fmt.Sprintf(BIP_BASE_PATH, ACCOUNT_INDEX)
	log.Printf("BIP Path %s", bip_path)

	path, err := bip32path.ParsePath(bip_path)

	curve := slip10.Secp256k1()
	key, err := slip10.DeriveKeyFromPath(seed, curve, path)
	if err != nil {
		fmt.Errorf("failed deriving %s key: %w", curve.Name(), err)
	}

	return iotaSeedFromKey(key), mnemonic
}

func randSeed() (string, bip39.Mnemonic) {
	var (
		err     error
		entropy []byte
	)

	if err != nil {
		fmt.Errorf("failed generating entropy: %w", err)
	}

	if MNEMONIC == nil {
		entropy, err = generateEntropy(256 / 8)

		MNEMONIC, _ = bip39.EntropyToMnemonic(entropy)
	}

	seed, _ := bip39.MnemonicToSeed(MNEMONIC, "")

	var bip_path = fmt.Sprintf(BIP_BASE_PATH, ACCOUNT_INDEX+seeds_count)
	log.Printf("BIP Path %s", bip_path)

	path, err := bip32path.ParsePath(bip_path)

	curve := slip10.Secp256k1()
	key, err := slip10.DeriveKeyFromPath(seed, curve, path)
	if err != nil {
		fmt.Errorf("failed deriving %s key: %w", curve.Name(), err)
	}

	seeds_count++

	return iotaSeedFromKey(key), MNEMONIC
}

func mnemonicToSeed(mnemonic bip39.Mnemonic) (trinary.Hash, error) {
	seedBytes, err := bip39.MnemonicToEntropy(mnemonic)
	if err != nil {
		return "", err
	}

	trytes, err := kerl.KerlBytesToTrytes(seedBytes)

	return trytes, err
}

func generateEntropy(size int) ([]byte, error) {
	entropy := make([]byte, size)
	if _, err := rand.Read(entropy); err != nil {
		return nil, err
	}
	return entropy, nil
}

func parseMnemonic(s string) bip39.Mnemonic {
	normalized := norm.NFKD.String(s)
	return strings.Fields(normalized)
}

func iotaSeedFromKey(key *slip10.Key) trinary.Hash {
	hash := sha3.NewLegacyKeccak384()

	hash.Write(key.Key[0:32])
	hash.Write(key.ChainCode[0:16])
	hash.Write(key.Key[16:32])
	hash.Write(key.ChainCode[0:32])

	seed, err := kerl.KerlBytesToTrytes(hash.Sum(nil))
	if err != nil {
		panic(err)
	}
	return seed
}
