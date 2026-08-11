import argparse
from tshark_shared.config import ingestor_settings
from ingestor.sources import LiveTsharkSource, PcapReplaySource
from ingestor.publisher import MQTTPublisher


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", choices=["live", "pcap"], required=True)
    parser.add_argument("--interface", default=ingestor_settings.TSHARK_INTERFACE)
    parser.add_argument("--bpf-filter", default=ingestor_settings.TSHARK_BPF_FILTER)
    parser.add_argument("--pcap", default=ingestor_settings.TSHARK_PCAP_PATH)
    parser.add_argument("--replay-loop", action="store_true", default=ingestor_settings.TSHARK_REPLAY_LOOP)
    parser.add_argument(
        "--rate",
        type=float,
        default=ingestor_settings.TSHARK_REPLAY_RATE,
        help="max packets/sec for pcap replay; 0 = unthrottled",
    )
    args = parser.parse_args()

    publisher = MQTTPublisher()
    publisher.start()

    if args.source == "live":
        source = LiveTsharkSource(args.interface, args.bpf_filter)
    else:
        source = PcapReplaySource(args.pcap, args.replay_loop, args.rate)

    try:
        for line in source.lines():
            publisher.publish_line(line)
    except KeyboardInterrupt:
        pass
    finally:
        publisher.stop()


if __name__ == "__main__":
    main()
