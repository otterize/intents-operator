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
        OUTPUT=$(kubectl logs --since=10s -n otterize-tutorial-npol $POD)
        grep "$OUTPUT_MESSAGE" <(echo $OUTPUT) && return || sleep 0.2;
        CURRENT_TIME=$(date +%s)
    done
    echo "Failed to find output in logs"
    echo "Last output was: $OUTPUT"
    echo "##############################################"
    echo "##############################################"
    echo "Operator manager logs:"
    kubectl logs -n otterize-system -l app=intents-operator --tail 400
    exit 1
}