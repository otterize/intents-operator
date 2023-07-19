 wait_for_log() {
            POD=$1
            DELAY_SECONDS=$2
            CURRENT_TIME=$(date +%s)
            END_TIME=$((CURRENT_TIME+DELAY_SECONDS))
            OUTPUT_MESSAGE="${@:3}"
            echo "Waiting for output: $OUTPUT_MESSAGE"
            echo "Waiting for $DELAY_SECONDS seconds until $END_TIME starting from $CURRENT_TIME"
            while [ $CURRENT_TIME -lt $END_TIME ];
            do
              grep "$OUTPUT_MESSAGE" <(kubectl logs --since=10s -n otterize-tutorial-npol $POD) && return || sleep 0.2;
              CURRENT_TIME=$(date +%s)
            done
            echo "Failed to find output in logs"
            exit 1
}
